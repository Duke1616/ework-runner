package artifact

import (
	"context"
	"errors"
	"fmt"

	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/errs"
	"gorm.io/gorm"
)

func (s *service) Active(ctx context.Context,
	target domain.ArtifactTarget) (*domain.ArtifactRelease, error) {
	if err := target.Validate(); err != nil {
		return nil, err
	}
	release, err := s.repo.FindActive(ctx, target)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询当前制品失败: %w", err)
	}
	return &release, nil
}

func (s *service) Status(ctx context.Context,
	target domain.ArtifactTarget) (domain.ArtifactStatus, error) {
	if err := target.Validate(); err != nil {
		return domain.ArtifactStatus{}, err
	}
	status := domain.ArtifactStatus{Target: target}
	if target.Scope == domain.CodebookScopeTenant {
		project, err := s.artifactProject(ctx, target.ProjectID)
		if err != nil {
			return domain.ArtifactStatus{}, err
		}
		status.SourceRevision = project.SourceRevision
	}
	active, err := s.Active(ctx, target)
	if err != nil {
		return domain.ArtifactStatus{}, err
	}
	status.Active = active
	status.PendingChanges = active != nil && target.Scope == domain.CodebookScopeTenant &&
		active.SourceRevision != status.SourceRevision
	return status, nil
}

func (s *service) List(ctx context.Context, target domain.ArtifactTarget,
	offset, limit int64) ([]domain.ArtifactRelease, int64, error) {
	if err := target.Validate(); err != nil {
		return nil, 0, err
	}
	if offset < 0 || limit <= 0 {
		return nil, 0, fmt.Errorf("%w: 分页参数非法: offset=%d limit=%d",
			errs.ErrInvalidParameter, offset, limit)
	}
	return s.repo.List(ctx, target, offset, limit)
}

func (s *service) Activate(ctx context.Context, target domain.ArtifactTarget, id int64) error {
	if _, err := s.validateWriteTarget(ctx, target); err != nil {
		return err
	}
	if id <= 0 {
		return fmt.Errorf("%w: 制品发布 ID 非法: %d", errs.ErrInvalidParameter, id)
	}
	return s.repo.Activate(ctx, target, id)
}

func (s *service) validateWriteTarget(ctx context.Context,
	target domain.ArtifactTarget) (int64, error) {
	tenantID := ctxutil.GetTenantID(ctx).Int64()
	if err := target.ValidateWriteAccess(tenantID, ctxutil.SystemTenantID); err != nil {
		return 0, err
	}
	if target.Scope == domain.CodebookScopeTenant {
		project, err := s.artifactProject(ctx, target.ProjectID)
		if err != nil {
			return 0, err
		}
		if err = project.ValidateWritable(); err != nil {
			return 0, err
		}
	}
	return tenantID, nil
}

func (s *service) artifactProject(ctx context.Context,
	projectID int64) (domain.CodebookProject, error) {
	project, err := s.repo.GetProject(ctx, projectID)
	if err != nil {
		return domain.CodebookProject{}, fmt.Errorf("查询代码项目失败: %w", err)
	}
	if !project.ArtifactEnabled {
		return domain.CodebookProject{}, fmt.Errorf("%w: 当前项目不是制品库", errs.ErrInvalidParameter)
	}
	if project.ArtifactNamespace == "" {
		return domain.CodebookProject{}, fmt.Errorf("%w: 制品库缺少导入命名空间", errs.ErrInvalidParameter)
	}
	return project, nil
}
