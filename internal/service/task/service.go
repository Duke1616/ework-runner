package task

import (
	"context"
	"fmt"

	"github.com/Duke1616/ework-runner/internal/domain"
	"github.com/Duke1616/ework-runner/internal/errs"
	"github.com/Duke1616/ework-runner/internal/repository"
)

// Service 任务服务接口
type Service interface {
	// Create 创建任务
	Create(ctx context.Context, task domain.Task) (domain.Task, error)
	// SchedulableTasks 获取可调度的任务列表，preemptedTimeoutMs 表示处于 PREEMPTED 状态任务的超时时间（毫秒）
	SchedulableTasks(ctx context.Context, preemptedTimeoutMs int64, limit int) ([]domain.Task, error)
	// UpdateNextTime 更新任务的下次执行时间
	UpdateNextTime(ctx context.Context, id int64) (domain.Task, error)
	// GetByID 根据ID获取task
	GetByID(ctx context.Context, id int64) (domain.Task, error)
}

type service struct {
	repo repository.TaskRepository
}

// NewService 创建任务服务实例
func NewService(repo repository.TaskRepository) Service {
	return &service{
		repo: repo,
	}
}

func (s *service) Create(ctx context.Context, task domain.Task) (domain.Task, error) {
	// 计算并设置下次执行时间
	nextTime, err := task.CalculateNextTime()
	if err != nil {
		return domain.Task{}, fmt.Errorf("%w: %w", errs.ErrInvalidTaskCronExpr, err)
	}
	if nextTime.IsZero() {
		return domain.Task{}, errs.ErrInvalidTaskCronExpr
	}
	task.NextTime = nextTime.UnixMilli()
	return s.repo.Create(ctx, task)
}

func (s *service) SchedulableTasks(ctx context.Context, preemptedTimeoutMs int64, limit int) ([]domain.Task, error) {
	return s.repo.SchedulableTasks(ctx, preemptedTimeoutMs, limit)
}

func (s *service) UpdateNextTime(ctx context.Context, id int64) (domain.Task, error) {
	task, err := s.GetByID(ctx, id)
	if err != nil {
		return domain.Task{}, err
	}
	// 计算并设置下次执行时间
	nextTime, err := task.CalculateNextTime()
	if err != nil {
		return domain.Task{}, fmt.Errorf("%w: %w", errs.ErrInvalidTaskCronExpr, err)
	}
	if nextTime.IsZero() {
		// 不需要继续执行了
		return task, nil
	}
	task.NextTime = nextTime.UnixMilli()
	return s.repo.UpdateNextTime(ctx, task.ID, task.Version, task.NextTime)
}

func (s *service) GetByID(ctx context.Context, id int64) (domain.Task, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *service) UpdateScheduleParams(ctx context.Context, task domain.Task, params map[string]string) (domain.Task, error) {
	task.UpdateScheduleParams(params)
	return s.repo.UpdateScheduleParams(ctx, task.ID, task.Version, task.ScheduleParams)
}
