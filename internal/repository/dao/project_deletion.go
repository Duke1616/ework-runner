package dao

import (
	"context"
	"fmt"
	"time"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/errs"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ProjectDeletionDAO 负责项目删除所需的跨表事务。
type ProjectDeletionDAO interface {
	// Preview 查询删除指定租户项目会影响的数据。
	Preview(ctx context.Context, projectID int64) (domain.ProjectDeleteImpact, error)
	// Delete 在事务中停止关联任务并删除项目数据。
	Delete(ctx context.Context, projectID int64, projectName string) (domain.ProjectDeleteCleanup, error)
}

type GORMProjectDeletionDAO struct{ db *gorm.DB }

func NewGORMProjectDeletionDAO(db *gorm.DB) ProjectDeletionDAO {
	return &GORMProjectDeletionDAO{db: db}
}

type projectDeletionSnapshot struct {
	project            CodebookProject
	nodeIDs            []int64
	versionCount       int64
	versionObjectKeys  []string
	artifactReleases   []ArtifactRelease
	projectSources     []ProjectSource
	retainedReleaseIDs map[int64]struct{}
	retainedSourceIDs  map[int64]struct{}
	taskIDs            []int64
	activeTaskCount    int64
	conversationIDs    []int64
}

func (g *GORMProjectDeletionDAO) Preview(ctx context.Context,
	projectID int64) (domain.ProjectDeleteImpact, error) {
	snapshot, err := g.loadSnapshot(ctx, projectID, false)
	if err != nil {
		return domain.ProjectDeleteImpact{}, err
	}
	if err = g.loadExecutionReferences(ctx, projectID, &snapshot); err != nil {
		return domain.ProjectDeleteImpact{}, err
	}
	return snapshot.impact(), nil
}

func (g *GORMProjectDeletionDAO) Delete(ctx context.Context, projectID int64,
	projectName string) (domain.ProjectDeleteCleanup, error) {
	var cleanup domain.ProjectDeleteCleanup
	err := g.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		dao := &GORMProjectDeletionDAO{db: tx}
		snapshot, err := dao.loadSnapshot(ctx, projectID, true)
		if err != nil {
			return err
		}
		if snapshot.project.Name != projectName {
			return fmt.Errorf("%w: 项目名称校验失败", errs.ErrInvalidParameter)
		}

		cleanup.ObjectKeys = append(cleanup.ObjectKeys, snapshot.versionObjectKeys...)
		if err = stopProjectTasks(tx, snapshot.taskIDs); err != nil {
			return err
		}
		// 任务停用后再确定历史执行引用，避免把已落库执行需要的不可变对象清掉。
		if err = dao.loadExecutionReferences(ctx, projectID, &snapshot); err != nil {
			return err
		}
		if len(snapshot.nodeIDs) > 0 {
			if err = tx.Where("node_id IN ?", snapshot.nodeIDs).Delete(&CodebookVersion{}).Error; err != nil {
				return err
			}
			if err = tx.Where("id IN ?", snapshot.nodeIDs).Delete(&Codebook{}).Error; err != nil {
				return err
			}
		}
		retainedObjectKeys := retainedProjectObjectKeys(snapshot)
		var releaseIDs []int64
		for _, release := range snapshot.artifactReleases {
			if _, keep := snapshot.retainedReleaseIDs[release.ID]; keep {
				continue
			}
			releaseIDs = append(releaseIDs, release.ID)
			if _, keep := retainedObjectKeys[release.ObjectKey]; release.ObjectKey != "" && !keep {
				cleanup.ObjectKeys = append(cleanup.ObjectKeys, release.ObjectKey)
			}
		}
		if len(releaseIDs) > 0 {
			deleted := tx.Where("id IN ? AND scope = ? AND project_id = ?",
				releaseIDs, domain.CodebookScopeTenant.String(), projectID).
				Delete(&ArtifactRelease{})
			if deleted.Error != nil {
				return deleted.Error
			}
		}

		var sourceIDs []int64
		for _, source := range snapshot.projectSources {
			if _, keep := snapshot.retainedSourceIDs[source.ID]; keep {
				continue
			}
			sourceIDs = append(sourceIDs, source.ID)
			if _, keep := retainedObjectKeys[source.ObjectKey]; source.ObjectKey != "" && !keep {
				cleanup.ObjectKeys = append(cleanup.ObjectKeys, source.ObjectKey)
			}
		}
		if len(sourceIDs) > 0 {
			deleted := tx.Where("id IN ? AND project_id = ?", sourceIDs, projectID).
				Delete(&ProjectSource{})
			if deleted.Error != nil {
				return deleted.Error
			}
		}

		if err = deleteProjectAI(tx, projectID, snapshot.conversationIDs); err != nil {
			return err
		}
		deleted := tx.Where("id = ? AND scope = ?", projectID,
			domain.CodebookScopeTenant.String()).Delete(&CodebookProject{})
		if deleted.Error != nil {
			return deleted.Error
		}
		if deleted.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
	return cleanup, err
}

func (g *GORMProjectDeletionDAO) loadSnapshot(ctx context.Context,
	projectID int64, lock bool) (projectDeletionSnapshot, error) {
	if projectID <= 0 {
		return projectDeletionSnapshot{}, fmt.Errorf("%w: 项目删除参数非法", errs.ErrInvalidParameter)
	}
	var project CodebookProject
	query := g.db.WithContext(ctx).Where("id = ? AND scope = ?", projectID,
		domain.CodebookScopeTenant.String())
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := query.First(&project).Error; err != nil {
		return projectDeletionSnapshot{}, err
	}

	var nodeIDs []int64
	if err := g.db.WithContext(ctx).Model(&Codebook{}).
		Where("scope = ? AND project_id = ?", domain.CodebookScopeTenant.String(), projectID).
		Pluck("id", &nodeIDs).Error; err != nil {
		return projectDeletionSnapshot{}, err
	}
	var versions []CodebookVersion
	if len(nodeIDs) > 0 {
		if err := g.db.WithContext(ctx).Select("id, storage_type, object_key").
			Where("node_id IN ?", nodeIDs).Find(&versions).Error; err != nil {
			return projectDeletionSnapshot{}, err
		}
	}
	var releases []ArtifactRelease
	if err := g.db.WithContext(ctx).Where("scope = ? AND project_id = ?",
		domain.CodebookScopeTenant.String(), projectID).Find(&releases).Error; err != nil {
		return projectDeletionSnapshot{}, err
	}
	var sources []ProjectSource
	if err := g.db.WithContext(ctx).Where("project_id = ?", projectID).
		Find(&sources).Error; err != nil {
		return projectDeletionSnapshot{}, err
	}
	taskIDs, activeTaskCount, err := g.findProjectTasks(ctx, nodeIDs, lock)
	if err != nil {
		return projectDeletionSnapshot{}, err
	}
	conversationIDs, err := g.findConversationIDs(ctx, projectID)
	if err != nil {
		return projectDeletionSnapshot{}, err
	}
	versionObjectKeys := make([]string, 0, len(versions))
	for _, version := range versions {
		if version.StorageType == domain.CodebookContentBlob.String() && version.ObjectKey != "" {
			versionObjectKeys = append(versionObjectKeys, version.ObjectKey)
		}
	}
	return projectDeletionSnapshot{
		project: project, nodeIDs: nodeIDs, versionCount: int64(len(versions)),
		versionObjectKeys: versionObjectKeys, artifactReleases: releases, projectSources: sources,
		taskIDs: taskIDs, activeTaskCount: activeTaskCount, conversationIDs: conversationIDs,
	}, nil
}

func (g *GORMProjectDeletionDAO) findProjectTasks(ctx context.Context,
	nodeIDs []int64, lock bool) ([]int64, int64, error) {
	if len(nodeIDs) == 0 {
		return nil, 0, nil
	}
	var rows []struct {
		ID     int64
		Status string
	}
	query := g.db.WithContext(ctx).Model(&Task{}).
		Select("id, status").
		Where(`CAST(COALESCE(
			program->>'$.project.entryCodebookId',
			program->>'$.inline.codebookId'
		) AS UNSIGNED) IN ?`, nodeIDs)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := query.Scan(&rows).Error; err != nil {
		return nil, 0, err
	}
	ids := make([]int64, 0, len(rows))
	var active int64
	for _, row := range rows {
		ids = append(ids, row.ID)
		if row.Status == string(domain.TaskStatusActive) || row.Status == string(domain.TaskStatusPreempted) {
			active++
		}
	}
	return ids, active, nil
}

func (g *GORMProjectDeletionDAO) findConversationIDs(ctx context.Context,
	projectID int64) ([]int64, error) {
	var ids []int64
	err := g.db.WithContext(ctx).Model(&AIConversation{}).
		Where("project_id = ?", projectID).Pluck("id", &ids).Error
	return ids, err
}

func (g *GORMProjectDeletionDAO) loadExecutionReferences(ctx context.Context,
	projectID int64, snapshot *projectDeletionSnapshot) error {
	retainedSources := make(map[int64]struct{})
	if len(snapshot.projectSources) > 0 {
		var rows []struct{ ID int64 }
		if err := g.db.WithContext(ctx).Model(&TaskExecution{}).
			Select("DISTINCT CAST(program->>'$.project.source.sourceId' AS UNSIGNED) AS id").
			Where("CAST(program->>'$.project.source.projectId' AS UNSIGNED) = ?", projectID).
			Scan(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			retainedSources[row.ID] = struct{}{}
		}
	}
	retainedReleases := make(map[int64]struct{})
	if len(snapshot.artifactReleases) > 0 {
		var rows []struct{ ID int64 }
		if err := g.db.WithContext(ctx).Model(&TaskExecution{}).
			Select("DISTINCT refs.release_id AS id").
			Joins(`JOIN JSON_TABLE(task_executions.artifact, '$[*]' COLUMNS (
				release_id BIGINT PATH '$.releaseId',
				project_id BIGINT PATH '$.projectId',
				scope VARCHAR(32) PATH '$.scope'
			)) AS refs ON TRUE`).
			Where("refs.scope = ? AND refs.project_id = ?",
				domain.CodebookScopeTenant.String(), projectID).
			Scan(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			retainedReleases[row.ID] = struct{}{}
		}
	}
	snapshot.retainedSourceIDs = intersectProjectSourceIDs(snapshot.projectSources, retainedSources)
	snapshot.retainedReleaseIDs = intersectArtifactReleaseIDs(snapshot.artifactReleases, retainedReleases)
	return nil
}

func intersectProjectSourceIDs(sources []ProjectSource, referenced map[int64]struct{}) map[int64]struct{} {
	result := make(map[int64]struct{})
	for _, source := range sources {
		if _, ok := referenced[source.ID]; ok {
			result[source.ID] = struct{}{}
		}
	}
	return result
}

func intersectArtifactReleaseIDs(releases []ArtifactRelease, referenced map[int64]struct{}) map[int64]struct{} {
	result := make(map[int64]struct{})
	for _, release := range releases {
		if _, ok := referenced[release.ID]; ok {
			result[release.ID] = struct{}{}
		}
	}
	return result
}

func stopProjectTasks(tx *gorm.DB, taskIDs []int64) error {
	if len(taskIDs) == 0 {
		return nil
	}
	updated := tx.Model(&Task{}).
		Where("id IN ? AND status IN ?", taskIDs,
			[]string{string(domain.TaskStatusActive), string(domain.TaskStatusPreempted)}).
		Updates(map[string]any{
			"status": string(domain.TaskStatusInactive), "next_time": 0,
			"schedule_node_id": nil, "version": gorm.Expr("version + 1"),
			"utime": time.Now().UnixMilli(),
		})
	return updated.Error
}

func retainedProjectObjectKeys(snapshot projectDeletionSnapshot) map[string]struct{} {
	keys := make(map[string]struct{})
	for _, release := range snapshot.artifactReleases {
		if _, keep := snapshot.retainedReleaseIDs[release.ID]; keep && release.ObjectKey != "" {
			keys[release.ObjectKey] = struct{}{}
		}
	}
	for _, source := range snapshot.projectSources {
		if _, keep := snapshot.retainedSourceIDs[source.ID]; keep && source.ObjectKey != "" {
			keys[source.ObjectKey] = struct{}{}
		}
	}
	return keys
}

func deleteProjectAI(tx *gorm.DB, projectID int64, conversationIDs []int64) error {
	if err := tx.Where("project_id = ?", projectID).
		Delete(&AISuggestion{}).Error; err != nil {
		return err
	}
	if len(conversationIDs) == 0 {
		return nil
	}
	if err := tx.Where("conversation_id IN ?", conversationIDs).
		Delete(&AIMessage{}).Error; err != nil {
		return err
	}
	return tx.Where("id IN ?", conversationIDs).
		Delete(&AIConversation{}).Error
}

func (snapshot projectDeletionSnapshot) impact() domain.ProjectDeleteImpact {
	impact := domain.ProjectDeleteImpact{
		TaskCount: int64(len(snapshot.taskIDs)), ActiveTaskCount: snapshot.activeTaskCount,
		CodebookNodeCount: int64(len(snapshot.nodeIDs)), CodebookVersionCount: snapshot.versionCount,
		ArtifactReleaseCount:         int64(len(snapshot.artifactReleases)),
		RetainedArtifactReleaseCount: int64(len(snapshot.retainedReleaseIDs)),
		ProjectSourceCount:           int64(len(snapshot.projectSources)),
		RetainedProjectSourceCount:   int64(len(snapshot.retainedSourceIDs)),
		AIConversationCount:          int64(len(snapshot.conversationIDs)),
	}
	for _, release := range snapshot.artifactReleases {
		impact.ArtifactReleaseBytes += release.Size
	}
	for _, source := range snapshot.projectSources {
		impact.ProjectSourceBytes += source.Size
	}
	return impact
}
