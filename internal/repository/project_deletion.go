package repository

import (
	"context"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/repository/dao"
)

//go:generate go tool mockgen -source=./project_deletion.go -package=repositorymocks -destination=./mocks/project_deletion.mock.go -typed

// ProjectDeletionRepository 提供项目删除所需的领域仓储能力。
type ProjectDeletionRepository interface {
	// Preview 查询删除项目会影响的数据。
	Preview(ctx context.Context, projectID int64) (domain.ProjectDeleteImpact, error)
	// Delete 删除项目数据并返回事务提交后需要清理的对象键。
	Delete(ctx context.Context, projectID int64, projectName string) (domain.ProjectDeleteCleanup, error)
}

type projectDeletionRepository struct {
	dao dao.ProjectDeletionDAO
}

// NewProjectDeletionRepository 创建项目删除仓储。
func NewProjectDeletionRepository(deletionDAO dao.ProjectDeletionDAO) ProjectDeletionRepository {
	return &projectDeletionRepository{dao: deletionDAO}
}

func (r *projectDeletionRepository) Preview(ctx context.Context,
	projectID int64) (domain.ProjectDeleteImpact, error) {
	return r.dao.Preview(ctx, projectID)
}

func (r *projectDeletionRepository) Delete(ctx context.Context,
	projectID int64, projectName string) (domain.ProjectDeleteCleanup, error) {
	return r.dao.Delete(ctx, projectID, projectName)
}
