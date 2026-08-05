package variable

import (
	"context"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/repository"
	"golang.org/x/sync/errgroup"
)

//go:generate go tool mockgen -source=./variable.go -package=variablemocks -destination=./mocks/variable.mock.go -typed

// Service 定义全局变量业务操作。
type Service interface {
	// Create 校验并创建全局变量。
	Create(ctx context.Context, req domain.Variable) (int64, error)
	// FindByID 根据主键 ID 获取全局变量。
	FindByID(ctx context.Context, id int64) (domain.Variable, error)
	// List 分页获取全局变量列表和总数。
	List(ctx context.Context, offset, limit int64, keyword string) ([]domain.Variable, int64, error)
	// Update 校验并更新全局变量。
	Update(ctx context.Context, req domain.Variable) (int64, error)
	// Delete 根据主键 ID 删除全局变量。
	Delete(ctx context.Context, id int64) (int64, error)
}

type service struct {
	repo repository.VariableRepository
}

// NewService 创建全局变量服务。
func NewService(repo repository.VariableRepository) Service {
	return &service{repo: repo}
}

// Create 校验并创建全局变量。
func (s *service) Create(ctx context.Context, req domain.Variable) (int64, error) {
	req.MarkGlobal()
	if err := req.Validate(); err != nil {
		return 0, err
	}
	return s.repo.CreateGlobalVariable(ctx, req)
}

// FindByID 根据主键 ID 获取全局变量。
func (s *service) FindByID(ctx context.Context, id int64) (domain.Variable, error) {
	if err := domain.ValidateVariableID(id); err != nil {
		return domain.Variable{}, err
	}
	return s.repo.FindGlobalVariable(ctx, id)
}

// List 分页获取全局变量列表和总数。
func (s *service) List(ctx context.Context, offset, limit int64, keyword string) ([]domain.Variable, int64, error) {
	var (
		eg    errgroup.Group
		res   []domain.Variable
		total int64
	)
	eg.Go(func() error {
		var err error
		res, err = s.repo.ListGlobalVariables(ctx, offset, limit, keyword)
		return err
	})
	eg.Go(func() error {
		var err error
		total, err = s.repo.CountGlobalVariables(ctx, keyword)
		return err
	})
	if err := eg.Wait(); err != nil {
		return nil, 0, err
	}
	return res, total, nil
}

// Update 校验并更新全局变量。
func (s *service) Update(ctx context.Context, req domain.Variable) (int64, error) {
	if err := req.ValidateID(); err != nil {
		return 0, err
	}
	req.MarkGlobal()
	if err := req.Validate(); err != nil {
		return 0, err
	}
	return s.repo.UpdateGlobalVariable(ctx, req)
}

// Delete 根据主键 ID 删除全局变量。
func (s *service) Delete(ctx context.Context, id int64) (int64, error) {
	if err := domain.ValidateVariableID(id); err != nil {
		return 0, err
	}
	return s.repo.DeleteGlobalVariable(ctx, id)
}
