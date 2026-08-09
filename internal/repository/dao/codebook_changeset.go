package dao

import (
	"context"
	"fmt"
	"path"
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
	NodeID                   int64
	ExpectedCurrentVersionID int64
	ExpectedHash             string
	Code                     string
	Message                  string
	SourceKey                string
}

type CodebookProjectChangeSet struct {
	ProjectID    int64
	BaseRevision int64
	Changes      []CodebookProjectChange
}

type CodebookProjectChangeResult struct {
	Path      string
	NodeID    int64
	VersionID int64
}

// ApplyProjectChangeSet 原子创建和更新项目文件，并且只递增一次源码修订号。
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
				node, exists := paths[key]
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
				updates = append(updates, change)
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
					Path: node.file.Path, NodeID: node.entity.ID, VersionID: node.versionID,
				}
			}
		}
		for _, change := range updates {
			versionID, err := applyCodebookUpdate(tx, ctx, change)
			if err != nil {
				return err
			}
			applied[strings.ToLower(change.Path)] = CodebookProjectChangeResult{
				Path: change.Path, NodeID: change.NodeID, VersionID: versionID,
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
			Path: change.Path, NodeID: node.entity.ID, VersionID: version.ID,
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
