package codebook

import (
	"context"
	"fmt"
	"strings"

	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/errs"
	"github.com/Duke1616/etask/internal/repository"
	"github.com/Duke1616/etask/pkg/blobstore"
	"golang.org/x/sync/errgroup"
)

// ProjectDeletionService 提供项目删除前的影响评估和删除能力。
type ProjectDeletionService interface {
	// Preview 查询删除项目会影响的数据。
	Preview(ctx context.Context, projectID int64) (domain.ProjectDeleteImpact, error)
	// Delete 校验项目名称，删除项目数据并清理对象存储。
	Delete(ctx context.Context, projectID int64, projectName string) error
}

type projectDeletionService struct {
	repo  repository.ProjectDeletionRepository
	store blobstore.Store
}

// NewProjectDeletionService 创建项目删除服务。
func NewProjectDeletionService(repo repository.ProjectDeletionRepository,
	store blobstore.Store) ProjectDeletionService {
	return &projectDeletionService{repo: repo, store: store}
}

func (s *projectDeletionService) Preview(ctx context.Context,
	projectID int64) (domain.ProjectDeleteImpact, error) {
	if ctxutil.GetTenantID(ctx).Int64() <= 0 || projectID <= 0 {
		return domain.ProjectDeleteImpact{}, fmt.Errorf("%w: 项目删除参数非法", errs.ErrInvalidParameter)
	}
	return s.repo.Preview(ctx, projectID)
}

func (s *projectDeletionService) Delete(ctx context.Context, projectID int64, projectName string) error {
	if ctxutil.GetTenantID(ctx).Int64() <= 0 || projectID <= 0 || strings.TrimSpace(projectName) == "" {
		return fmt.Errorf("%w: 项目删除参数非法", errs.ErrInvalidParameter)
	}
	cleanup, err := s.repo.Delete(ctx, projectID, projectName)
	if err != nil {
		return err
	}
	if err = s.deleteObjects(context.WithoutCancel(ctx), cleanup.ObjectKeys); err != nil {
		return fmt.Errorf("项目已删除，但清理对象存储失败: %w", err)
	}
	return nil
}

func (s *projectDeletionService) deleteObjects(ctx context.Context, keys []string) error {
	var group errgroup.Group
	group.SetLimit(8)
	for _, key := range uniqueObjectKeys(keys) {
		group.Go(func() error {
			if err := s.store.Delete(ctx, key); err != nil {
				return fmt.Errorf("删除对象 %s 失败: %w", key, err)
			}
			return nil
		})
	}
	return group.Wait()
}

func uniqueObjectKeys(keys []string) []string {
	seen := make(map[string]struct{}, len(keys))
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, key)
	}
	return result
}
