package runner

import (
	"context"
	"errors"
	"fmt"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/errs"
	"github.com/Duke1616/etask/internal/repository"
	"golang.org/x/sync/errgroup"
)

//go:generate go tool mockgen -source=./runner.go -package=runnermocks -destination=./mocks/runner.mock.go -typed

// Service 定义脚本执行单元业务操作。
type Service interface {
	// Create 校验并创建执行单元。
	Create(ctx context.Context, req domain.Runner) (int64, error)
	// Update 校验并更新执行单元。
	Update(ctx context.Context, req domain.Runner) (int64, error)
	// FindByID 根据主键 ID 获取执行单元。
	FindByID(ctx context.Context, id int64) (domain.Runner, error)
	// Delete 根据主键 ID 删除执行单元。
	Delete(ctx context.Context, id int64) (int64, error)
	// List 分页获取执行单元列表和总数。
	List(ctx context.Context, offset, limit int64, keyword, kind string) ([]domain.Runner, int64, error)
	// FindByCodebookIDAndTag 根据脚本模板 ID 和标签获取执行单元。
	FindByCodebookIDAndTag(ctx context.Context, codebookID int64, tag string) (domain.Runner, error)
	// ListByCodebookID 获取绑定指定脚本模板 ID 的全部执行单元。
	ListByCodebookID(ctx context.Context, codebookID int64) ([]domain.Runner, error)
	// ListExcludeCodebookID 获取未绑定指定脚本模板 ID 的执行单元列表。
	ListExcludeCodebookID(ctx context.Context, offset, limit int64, codebookID int64, keyword, kind string) ([]domain.Runner, int64, error)
	// ListByCodebookIDs 获取绑定任一脚本模板 ID 的执行单元列表。
	ListByCodebookIDs(ctx context.Context, codebookIDs []int64) ([]domain.Runner, error)
	// ListByIDs 根据 ID 列表批量获取执行单元。
	ListByIDs(ctx context.Context, ids []int64) ([]domain.Runner, error)
	// ListMergedVariables 获取执行单元变量，私有变量覆盖全局变量。
	ListMergedVariables(ctx context.Context, id int64) ([]domain.RunnerVariable, error)
}

type service struct {
	repo  repository.RunnerRepository
	pools repository.ExecutionPoolRepository
}

// NewService 创建执行单元服务。
func NewService(repo repository.RunnerRepository, pools repository.ExecutionPoolRepository) Service {
	return &service{repo: repo, pools: pools}
}

// Create 校验并创建执行单元。
func (s *service) Create(ctx context.Context, req domain.Runner) (int64, error) {
	if err := req.Validate(); err != nil {
		return 0, err
	}
	if err := s.normalizeTarget(ctx, &req); err != nil {
		return 0, err
	}
	if req.Action == 0 {
		req.Action = domain.RunnerActionRegistered
	}
	return s.repo.Create(ctx, req)
}

// Update 校验并更新执行单元。
func (s *service) Update(ctx context.Context, req domain.Runner) (int64, error) {
	if req.ID <= 0 {
		return 0, fmt.Errorf("%w: id = %d", errs.ErrInvalidParameter, req.ID)
	}
	if err := req.Validate(); err != nil {
		return 0, err
	}
	if err := s.normalizeTarget(ctx, &req); err != nil {
		return 0, err
	}
	return s.repo.Update(ctx, req)
}

func (s *service) normalizeTarget(ctx context.Context, runner *domain.Runner) error {
	pool, err := s.pools.Find(ctx, runner.Target)
	if err != nil {
		if errors.Is(err, repository.ErrExecutionPoolNotFound) {
			return fmt.Errorf("%w: execution pool %q not found", errs.ErrInvalidParameter, runner.Target)
		}
		return fmt.Errorf("查询执行资源池失败: %w", err)
	}
	if pool.Transport != runner.Kind.Transport() {
		return fmt.Errorf("%w: runner kind %s does not match execution pool transport %s",
			errs.ErrInvalidParameter, runner.Kind, pool.Transport)
	}
	runner.Target = pool.Name
	return nil
}

// FindByID 根据主键 ID 获取执行单元。
func (s *service) FindByID(ctx context.Context, id int64) (domain.Runner, error) {
	if id <= 0 {
		return domain.Runner{}, fmt.Errorf("%w: id = %d", errs.ErrInvalidParameter, id)
	}
	return s.repo.FindByID(ctx, id)
}

// Delete 根据主键 ID 删除执行单元。
func (s *service) Delete(ctx context.Context, id int64) (int64, error) {
	if id <= 0 {
		return 0, fmt.Errorf("%w: id = %d", errs.ErrInvalidParameter, id)
	}
	return s.repo.Delete(ctx, id)
}

// List 分页获取执行单元列表和总数。
func (s *service) List(ctx context.Context, offset, limit int64, keyword, kind string) ([]domain.Runner, int64, error) {
	var (
		eg    errgroup.Group
		res   []domain.Runner
		total int64
	)
	eg.Go(func() error {
		var err error
		res, err = s.repo.List(ctx, offset, limit, keyword, kind)
		return err
	})
	eg.Go(func() error {
		var err error
		total, err = s.repo.Count(ctx, keyword, kind)
		return err
	})
	if err := eg.Wait(); err != nil {
		return nil, 0, err
	}
	return res, total, nil
}

// FindByCodebookIDAndTag 根据脚本模板 ID 和标签获取执行单元。
func (s *service) FindByCodebookIDAndTag(ctx context.Context, codebookID int64, tag string) (domain.Runner, error) {
	if codebookID <= 0 {
		return domain.Runner{}, fmt.Errorf("%w: codebook_id = %d", errs.ErrInvalidParameter, codebookID)
	}
	if tag == "" {
		return domain.Runner{}, fmt.Errorf("%w: tag is empty", errs.ErrInvalidParameter)
	}
	return s.repo.FindByCodebookIDAndTag(ctx, codebookID, tag)
}

// ListByCodebookID 获取绑定指定脚本模板 ID 的全部执行单元。
func (s *service) ListByCodebookID(ctx context.Context, codebookID int64) ([]domain.Runner, error) {
	if codebookID <= 0 {
		return nil, fmt.Errorf("%w: codebook_id = %d", errs.ErrInvalidParameter, codebookID)
	}
	return s.repo.ListByCodebookID(ctx, codebookID)
}

// ListExcludeCodebookID 获取未绑定指定脚本模板 ID 的执行单元列表。
func (s *service) ListExcludeCodebookID(ctx context.Context, offset, limit int64, codebookID int64, keyword, kind string) ([]domain.Runner, int64, error) {
	if codebookID <= 0 {
		return nil, 0, fmt.Errorf("%w: codebook_id = %d", errs.ErrInvalidParameter, codebookID)
	}
	var (
		eg    errgroup.Group
		res   []domain.Runner
		total int64
	)
	eg.Go(func() error {
		var err error
		res, err = s.repo.ListExcludeCodebookID(ctx, offset, limit, codebookID, keyword, kind)
		return err
	})
	eg.Go(func() error {
		var err error
		total, err = s.repo.CountExcludeCodebookID(ctx, codebookID, keyword, kind)
		return err
	})
	if err := eg.Wait(); err != nil {
		return nil, 0, err
	}
	return res, total, nil
}

// ListByCodebookIDs 获取绑定任一脚本模板 ID 的执行单元列表。
func (s *service) ListByCodebookIDs(ctx context.Context, codebookIDs []int64) ([]domain.Runner, error) {
	return s.repo.ListByCodebookIDs(ctx, codebookIDs)
}

// ListByIDs 根据 ID 列表批量获取执行单元。
func (s *service) ListByIDs(ctx context.Context, ids []int64) ([]domain.Runner, error) {
	return s.repo.ListByIDs(ctx, ids)
}

func (s *service) ListMergedVariables(ctx context.Context, id int64) ([]domain.RunnerVariable, error) {
	if id <= 0 {
		return nil, fmt.Errorf("%w: id = %d", errs.ErrInvalidParameter, id)
	}
	return s.repo.ListMergedVariables(ctx, id)
}
