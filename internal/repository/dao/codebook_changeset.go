package dao

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/errs"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type CodebookProjectChange struct {
	Operation                string
	Path                     string
	SourcePath               string
	NodeID                   int64
	ExpectedCurrentVersionID int64
	ExpectedHash             string
	Code                     string
	Message                  string
	SourceKey                string
	CleanupObjectKeys        []string
}

type CodebookProjectChangeSet struct {
	ProjectID    int64
	BaseRevision int64
	Changes      []CodebookProjectChange
}

type CodebookProjectChangeResult struct {
	Path              string
	SourcePath        string
	Operation         string
	NodeID            int64
	VersionID         int64
	CleanupObjectKeys []string
}

// ApplyProjectChangeSet 原子创建、更新、重命名或删除项目文件，并且只递增一次源码修订号。
func (g *GORMCodebookDAO) ApplyProjectChangeSet(ctx context.Context,
	request CodebookProjectChangeSet) ([]CodebookProjectChangeResult, error) {
	var result []CodebookProjectChangeResult
	err := g.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var project CodebookProject
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND scope = ? AND status = ?", request.ProjectID,
				domain.CodebookScopeTenant.String(), domain.CodebookProjectStatusNormal.String()).
			First(&project).Error; err != nil {
			return err
		}
		var existing []Codebook
		if err := tx.Where("project_id = ? AND scope = ?", request.ProjectID,
			domain.CodebookScopeTenant.String()).Find(&existing).Error; err != nil {
			return err
		}
		root, _ := buildCodebookImportTree(existing)
		paths := indexCodebookImportPaths(root)
		if project.SourceRevision != request.BaseRevision {
			applied, ok, err := alreadyAppliedProjectChangeSet(tx, request, paths)
			if err != nil {
				return err
			}
			if !ok {
				return errs.ErrCodebookVersionConflict
			}
			result = applied
			return nil
		}
		creates := make([]CodebookImportFile, 0)
		updates := make([]CodebookProjectChange, 0)
		renames := make([]CodebookProjectChange, 0)
		deletes := make([]CodebookProjectChange, 0)
		for _, change := range request.Changes {
			key := strings.ToLower(change.Path)
			switch change.Operation {
			case domain.CodebookChangeOperationCreate.String():
				if _, exists := paths[key]; exists {
					return errs.ErrCodebookNameConflict
				}
				creates = append(creates, CodebookImportFile{
					Path: change.Path, Code: change.Code,
					StorageType: domain.CodebookContentInline.String(), Size: int64(len(change.Code)),
					ContentType: "text/plain; charset=utf-8", Hash: hashCode(change.Code),
					Message: change.Message, SourceKey: change.SourceKey,
				})
			case domain.CodebookChangeOperationUpdate.String():
				if err := validateExistingFileChange(tx, paths, change.Path, change); err != nil {
					return err
				}
				updates = append(updates, change)
			case domain.CodebookChangeOperationRename.String():
				if _, exists := paths[key]; exists {
					return errs.ErrCodebookNameConflict
				}
				if err := validateExistingFileChange(tx, paths, change.SourcePath, change); err != nil {
					return err
				}
				renames = append(renames, change)
			case domain.CodebookChangeOperationDelete.String():
				if err := validateExistingFileChange(tx, paths, change.Path, change); err != nil {
					return err
				}
				deletes = append(deletes, change)
			default:
				return fmt.Errorf("unsupported Codebook project change operation: %s", change.Operation)
			}
		}

		applied := make(map[string]CodebookProjectChangeResult, len(request.Changes))
		if len(creates) > 0 {
			plan, err := buildCodebookImportPlan(CodebookImport{
				ProjectID: request.ProjectID, Files: creates,
			}, existing, time.Now().UnixMilli())
			if err != nil {
				return err
			}
			if err = plan.persist(tx, codebookAuthorUserID(ctx)); err != nil {
				return err
			}
			for _, node := range plan.files {
				applied[strings.ToLower(node.file.Path)] = CodebookProjectChangeResult{
					Path: node.file.Path, Operation: domain.CodebookChangeOperationCreate.String(),
					NodeID: node.entity.ID, VersionID: node.versionID,
				}
			}
		}
		for _, change := range updates {
			versionID, err := applyCodebookUpdate(tx, ctx, change)
			if err != nil {
				return err
			}
			applied[strings.ToLower(change.Path)] = CodebookProjectChangeResult{
				Path: change.Path, Operation: change.Operation,
				NodeID: change.NodeID, VersionID: versionID,
			}
		}
		for _, change := range renames {
			versionID, err := applyCodebookRename(tx, change)
			if err != nil {
				return err
			}
			applied[strings.ToLower(change.Path)] = CodebookProjectChangeResult{
				Path: change.Path, SourcePath: change.SourcePath, Operation: change.Operation,
				NodeID: change.NodeID, VersionID: versionID,
			}
		}
		for _, change := range deletes {
			objectKeys, err := applyCodebookDelete(tx, change)
			if err != nil {
				return err
			}
			applied[strings.ToLower(change.Path)] = CodebookProjectChangeResult{
				Path: change.Path, Operation: change.Operation, NodeID: change.NodeID,
				CleanupObjectKeys: objectKeys,
			}
		}
		result = make([]CodebookProjectChangeResult, 0, len(request.Changes))
		for _, change := range request.Changes {
			result = append(result, applied[strings.ToLower(change.Path)])
		}
		return bumpProjectSourceRevision(tx, domain.CodebookScopeTenant.String(), request.ProjectID)
	})
	return result, codebookWriteError(err)
}

func alreadyAppliedProjectChangeSet(tx *gorm.DB, request CodebookProjectChangeSet,
	paths map[string]*codebookImportNode) ([]CodebookProjectChangeResult, bool, error) {
	result := make([]CodebookProjectChangeResult, 0, len(request.Changes))
	for _, change := range request.Changes {
		node, exists := paths[strings.ToLower(change.Path)]
		if change.Operation == domain.CodebookChangeOperationDelete.String() {
			var count int64
			if err := tx.Model(&Codebook{}).Where("id = ?", change.NodeID).Count(&count).Error; err != nil {
				return nil, false, err
			}
			if exists || count > 0 {
				return nil, false, nil
			}
			result = append(result, CodebookProjectChangeResult{
				Path: change.Path, Operation: change.Operation, NodeID: change.NodeID,
				CleanupObjectKeys: change.CleanupObjectKeys,
			})
			continue
		}
		if change.Operation == domain.CodebookChangeOperationRename.String() {
			if !exists || node.entity.ID != change.NodeID ||
				node.entity.CurrentVersionID != change.ExpectedCurrentVersionID {
				return nil, false, nil
			}
			result = append(result, CodebookProjectChangeResult{
				Path: change.Path, SourcePath: change.SourcePath, Operation: change.Operation,
				NodeID:    node.entity.ID,
				VersionID: node.entity.CurrentVersionID,
			})
			continue
		}
		if !exists || node.entity.Kind != domain.CodebookKindFile.String() ||
			node.entity.CurrentVersionID == 0 || change.SourceKey == "" {
			return nil, false, nil
		}
		var version CodebookVersion
		if err := tx.Where("id = ? AND node_id = ?", node.entity.CurrentVersionID,
			node.entity.ID).First(&version).Error; err != nil {
			return nil, false, err
		}
		if version.SourceKey == nil || *version.SourceKey != change.SourceKey ||
			version.Hash != hashCode(change.Code) {
			return nil, false, nil
		}
		result = append(result, CodebookProjectChangeResult{
			Path: change.Path, Operation: change.Operation,
			NodeID: node.entity.ID, VersionID: version.ID,
		})
	}
	return result, true, nil
}

func applyCodebookUpdate(tx *gorm.DB, ctx context.Context,
	change CodebookProjectChange) (int64, error) {
	var maxVersionNo int64
	if err := tx.Model(&CodebookVersion{}).Where("node_id = ?", change.NodeID).
		Select("COALESCE(MAX(version_no), 0)").Scan(&maxVersionNo).Error; err != nil {
		return 0, err
	}
	sourceKey := change.SourceKey
	version := CodebookVersion{
		NodeID: change.NodeID, Scope: domain.CodebookScopeTenant.String(),
		VersionNo: maxVersionNo + 1, Code: change.Code,
		StorageType: domain.CodebookContentInline.String(), Size: int64(len(change.Code)),
		ContentType: "text/plain; charset=utf-8", Hash: hashCode(change.Code),
		Message: change.Message, AuthorUserID: codebookAuthorUserID(ctx),
		CTime: time.Now().UnixMilli(),
	}
	if sourceKey != "" {
		version.SourceKey = &sourceKey
	}
	if err := tx.Create(&version).Error; err != nil {
		return 0, err
	}
	if err := tx.Model(&Codebook{}).Where("id = ?", change.NodeID).
		Updates(map[string]any{
			"current_version_id": version.ID, "utime": time.Now().UnixMilli(),
		}).Error; err != nil {
		return 0, err
	}
	return version.ID, nil
}

func validateExistingFileChange(tx *gorm.DB, paths map[string]*codebookImportNode,
	filePath string, change CodebookProjectChange) error {
	node, exists := paths[strings.ToLower(filePath)]
	if !exists || node.entity.ID != change.NodeID ||
		node.entity.Kind != domain.CodebookKindFile.String() ||
		node.entity.CurrentVersionID != change.ExpectedCurrentVersionID {
		return errs.ErrCodebookVersionConflict
	}
	var version CodebookVersion
	if err := tx.Where("id = ? AND node_id = ?", change.ExpectedCurrentVersionID,
		change.NodeID).First(&version).Error; err != nil {
		return err
	}
	actualHash := version.Hash
	if actualHash == "" {
		actualHash = hashCode(version.Code)
	}
	if actualHash != change.ExpectedHash {
		return errs.ErrCodebookVersionConflict
	}
	return nil
}

func applyCodebookRename(tx *gorm.DB, change CodebookProjectChange) (int64, error) {
	if err := renameCodebookNode(tx, change.NodeID, path.Base(change.Path),
		time.Now().UnixMilli()); err != nil {
		return 0, err
	}
	return change.ExpectedCurrentVersionID, nil
}

func applyCodebookDelete(tx *gorm.DB, change CodebookProjectChange) ([]string, error) {
	var objectKeys []string
	if err := tx.Model(&CodebookVersion{}).
		Where("node_id = ? AND storage_type = ? AND object_key <> ''", change.NodeID,
			domain.CodebookContentBlob.String()).
		Distinct("object_key").Pluck("object_key", &objectKeys).Error; err != nil {
		return nil, err
	}
	sort.Strings(objectKeys)
	expectedKeys := append([]string(nil), change.CleanupObjectKeys...)
	sort.Strings(expectedKeys)
	if !equalStrings(objectKeys, expectedKeys) {
		return nil, errs.ErrCodebookVersionConflict
	}
	if err := tx.Where("node_id = ?", change.NodeID).Delete(&CodebookVersion{}).Error; err != nil {
		return nil, err
	}
	result := tx.Where("id = ?", change.NodeID).Delete(&Codebook{})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return objectKeys, nil
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func indexCodebookImportPaths(root *codebookImportNode) map[string]*codebookImportNode {
	result := make(map[string]*codebookImportNode)
	var walk func(*codebookImportNode, string)
	walk = func(parent *codebookImportNode, parentPath string) {
		for _, child := range parent.children {
			childPath := path.Join(parentPath, child.entity.Name)
			result[strings.ToLower(childPath)] = child
			walk(child, childPath)
		}
	}
	walk(root, "")
	return result
}
