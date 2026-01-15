package repository

import (
	"context"
	"database/sql"

	"github.com/Duke1616/ework-runner/internal/domain"
	"github.com/Duke1616/ework-runner/internal/repository/dao"
	"github.com/Duke1616/ework-runner/pkg/sqlx"
	"github.com/ecodeclub/ekit/slice"
)

type TaskRepository interface {
	// Create 创建任务
	Create(ctx context.Context, task domain.Task) (domain.Task, error)
	// GetByID 根据ID获取任务
	GetByID(ctx context.Context, id int64) (domain.Task, error)
	// SchedulableTasks 获取可调度的任务列表，preemptedTimeoutMs 表示处于 PREEMPTED 状态任务的超时时间（毫秒）
	SchedulableTasks(ctx context.Context, preemptedTimeoutMs int64, limit int) ([]domain.Task, error)
	// Acquire 抢占任务
	Acquire(ctx context.Context, id, version int64, scheduleNodeID string) (domain.Task, error)
	// Release 释放任务
	Release(ctx context.Context, id int64, scheduleNodeID string) (domain.Task, error)
	// Renew 续约所有抢占到的任务
	Renew(ctx context.Context, scheduleNodeID string) error
	// UpdateNextTime 更新任务的下次执行时间
	UpdateNextTime(ctx context.Context, id, version, nextTime int64) (domain.Task, error)
	// UpdateScheduleParams 更新调度参数
	UpdateScheduleParams(ctx context.Context, id, version int64, scheduleParams map[string]string) (domain.Task, error)
	// FindByPlanID 根据计划ID获取所有子任务
	FindByPlanID(ctx context.Context, planID int64) ([]domain.Task, error)
}

type taskRepository struct {
	dao dao.TaskDAO
}

func NewTaskRepository(taskDAO dao.TaskDAO) TaskRepository {
	return &taskRepository{dao: taskDAO}
}

func (r *taskRepository) FindByPlanID(ctx context.Context, planID int64) ([]domain.Task, error) {
	daoTasks, err := r.dao.FindByPlanID(ctx, planID)
	if err != nil {
		return nil, err
	}
	return slice.Map(daoTasks, func(_ int, src *dao.Task) domain.Task {
		return r.toDomain(src)
	}), nil
}

func (r *taskRepository) Create(ctx context.Context, task domain.Task) (domain.Task, error) {
	created, err := r.dao.Create(ctx, r.toEntity(task))
	if err != nil {
		return domain.Task{}, err
	}
	return r.toDomain(created), nil
}

func (r *taskRepository) GetByID(ctx context.Context, id int64) (domain.Task, error) {
	daoTask, err := r.dao.GetByID(ctx, id)
	if err != nil {
		return domain.Task{}, err
	}
	return r.toDomain(daoTask), nil
}

func (r *taskRepository) SchedulableTasks(ctx context.Context, preemptedTimeoutMs int64, limit int) ([]domain.Task, error) {
	tasks, err := r.dao.FindSchedulableTasks(ctx, preemptedTimeoutMs, limit)
	if err != nil {
		return nil, err
	}
	return slice.Map(tasks, func(_ int, src *dao.Task) domain.Task {
		return r.toDomain(src)
	}), nil
}

func (r *taskRepository) Acquire(ctx context.Context, id, version int64, scheduleNodeID string) (domain.Task, error) {
	task, err := r.dao.Acquire(ctx, id, version, scheduleNodeID)
	if err != nil {
		return domain.Task{}, err
	}
	return r.toDomain(task), nil
}

func (r *taskRepository) Release(ctx context.Context, id int64, scheduleNodeID string) (domain.Task, error) {
	task, err := r.dao.Release(ctx, id, scheduleNodeID)
	if err != nil {
		return domain.Task{}, err
	}
	return r.toDomain(task), nil
}

func (r *taskRepository) Renew(ctx context.Context, scheduleNodeID string) error {
	return r.dao.Renew(ctx, scheduleNodeID)
}

func (r *taskRepository) UpdateNextTime(ctx context.Context, id, version, nextTime int64) (domain.Task, error) {
	task, err := r.dao.UpdateNextTime(ctx, id, version, nextTime)
	if err != nil {
		return domain.Task{}, err
	}
	return r.toDomain(task), nil
}

func (r *taskRepository) UpdateScheduleParams(ctx context.Context, id, version int64, scheduleParams map[string]string) (domain.Task, error) {
	task, err := r.dao.UpdateScheduleParams(ctx, id, version, scheduleParams)
	if err != nil {
		return domain.Task{}, err
	}
	return r.toDomain(task), nil
}

// toEntity 将领域模型转换为DAO模型
func (r *taskRepository) toEntity(task domain.Task) dao.Task {
	var scheduleNodeID sql.NullString
	if task.ScheduleNodeID != "" {
		scheduleNodeID = sql.NullString{String: task.ScheduleNodeID, Valid: true}
	}

	var grpcConfig sqlx.JSONColumn[domain.GrpcConfig]
	if task.GrpcConfig != nil {
		grpcConfig = sqlx.JSONColumn[domain.GrpcConfig]{Val: *task.GrpcConfig, Valid: true}
	}

	var httpConfig sqlx.JSONColumn[domain.HTTPConfig]
	if task.HTTPConfig != nil {
		httpConfig = sqlx.JSONColumn[domain.HTTPConfig]{Val: *task.HTTPConfig, Valid: true}
	}

	var retryConfig sqlx.JSONColumn[domain.RetryConfig]
	if task.RetryConfig != nil {
		retryConfig = sqlx.JSONColumn[domain.RetryConfig]{Val: *task.RetryConfig, Valid: true}
	}

	var scheduleParams sqlx.JSONColumn[map[string]string]
	if task.ScheduleParams != nil {
		scheduleParams = sqlx.JSONColumn[map[string]string]{Val: task.ScheduleParams, Valid: true}
	}

	return dao.Task{
		ID:                  task.ID,
		Name:                task.Name,
		CronExpr:            task.CronExpr,
		GrpcConfig:          grpcConfig,
		HTTPConfig:          httpConfig,
		RetryConfig:         retryConfig,
		ScheduleParams:      scheduleParams,
		MaxExecutionSeconds: task.MaxExecutionSeconds,
		ScheduleNodeID:      scheduleNodeID,
		NextTime:            task.NextTime,
		Status:              task.Status.String(),
		Version:             task.Version,
		Ctime:               task.CTime,
		Utime:               task.UTime,
	}
}

// toDomain 将DAO模型转换为领域模型
func (r *taskRepository) toDomain(daoTask *dao.Task) domain.Task {
	var scheduleNodeID string
	if daoTask.ScheduleNodeID.Valid {
		scheduleNodeID = daoTask.ScheduleNodeID.String
	}

	var grpcConfig *domain.GrpcConfig
	if daoTask.GrpcConfig.Valid {
		grpcConfig = &daoTask.GrpcConfig.Val
	}

	var httpConfig *domain.HTTPConfig
	if daoTask.HTTPConfig.Valid {
		httpConfig = &daoTask.HTTPConfig.Val
	}

	var retryConfig *domain.RetryConfig
	if daoTask.RetryConfig.Valid {
		retryConfig = &daoTask.RetryConfig.Val
	}

	var scheduleParams map[string]string
	if daoTask.ScheduleParams.Valid {
		scheduleParams = daoTask.ScheduleParams.Val
	}

	return domain.Task{
		ID:                  daoTask.ID,
		Name:                daoTask.Name,
		CronExpr:            daoTask.CronExpr,
		GrpcConfig:          grpcConfig,
		HTTPConfig:          httpConfig,
		RetryConfig:         retryConfig,
		MaxExecutionSeconds: daoTask.MaxExecutionSeconds,
		ScheduleParams:      scheduleParams,
		ScheduleNodeID:      scheduleNodeID,
		NextTime:            daoTask.NextTime,
		Status:              domain.TaskStatus(daoTask.Status),
		Version:             daoTask.Version,
		UTime:               daoTask.Utime,
		CTime:               daoTask.Ctime,
	}
}
