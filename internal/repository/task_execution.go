package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Duke1616/ework-runner/internal/domain"
	"github.com/Duke1616/ework-runner/internal/repository/dao"
	"github.com/Duke1616/ework-runner/pkg/sqlx"
	"github.com/ecodeclub/ekit/slice"
)

type TaskExecutionRepository interface {
	// Create 创建任务执行实例
	Create(ctx context.Context, execution domain.TaskExecution) (domain.TaskExecution, error)
	// UpdateStatus 更新执行状态
	UpdateStatus(ctx context.Context, id int64, status domain.TaskExecutionStatus) error
	// GetByID 根据ID获取执行实例
	GetByID(ctx context.Context, id int64) (domain.TaskExecution, error)
	// FindRetryableExecutions 查找所有可以重试的执行记录
	// limit: 查询结果数量限制
	FindRetryableExecutions(ctx context.Context, limit int) ([]domain.TaskExecution, error)
	// UpdateRetryResult 更新重试结果
	UpdateRetryResult(ctx context.Context, id, retryCount, nextRetryTime int64, status domain.TaskExecutionStatus, progress int32, endTime int64, scheduleParams map[string]string, executorNodeID string) error
	// SetRunningState 设置任务为运行状态并更新进度
	SetRunningState(ctx context.Context, id int64, progress int32, executorNodeID string) error
	// UpdateRunningProgress 更新任务执行进度（仅在RUNNING状态下有效）
	UpdateRunningProgress(ctx context.Context, id int64, progress int32) error
	// UpdateScheduleResult 更新调度结果
	UpdateScheduleResult(ctx context.Context, id int64, status domain.TaskExecutionStatus, progress int32, endTime int64, scheduleParams map[string]string, executorNodeID string) error
	// FindReschedulableExecutions 查找所有可以重调度的执行记录
	FindReschedulableExecutions(ctx context.Context, limit int) ([]domain.TaskExecution, error)

	FindExecutionsByPlanExecID(ctx context.Context, planExecID int64) (map[int64]domain.TaskExecution, error)
	FindByTaskID(ctx context.Context, taskID int64) ([]domain.TaskExecution, error)
	FindExecutionByTaskIDAndPlanExecID(ctx context.Context, taskID int64, planExecID int64) (domain.TaskExecution, error)
	// FindTimeoutExecutions 查找超时的执行记录
	FindTimeoutExecutions(ctx context.Context, limit int) ([]domain.TaskExecution, error)
}

type taskExecutionRepository struct {
	dao      dao.TaskExecutionDAO
	taskRepo TaskRepository
}

func NewTaskExecutionRepository(executionDAO dao.TaskExecutionDAO, taskRepo TaskRepository) TaskExecutionRepository {
	return &taskExecutionRepository{
		dao:      executionDAO,
		taskRepo: taskRepo,
	}
}

func (r *taskExecutionRepository) FindExecutionByTaskIDAndPlanExecID(ctx context.Context, taskID, planExecID int64) (domain.TaskExecution, error) {
	daoExec, err := r.dao.FindExecutionByTaskIDAndPlanExecID(ctx, taskID, planExecID)
	if err != nil {
		return domain.TaskExecution{}, err
	}
	return r.toDomain(daoExec), nil
}

func (r *taskExecutionRepository) FindByTaskID(ctx context.Context, taskID int64) ([]domain.TaskExecution, error) {
	daoExecutions, err := r.dao.FindByTaskID(ctx, taskID)
	if err != nil {
		return nil, err
	}
	return slice.Map(daoExecutions, func(_ int, src dao.TaskExecution) domain.TaskExecution {
		return r.toDomain(src)
	}), nil
}

func (r *taskExecutionRepository) FindExecutionsByPlanExecID(ctx context.Context, planExecID int64) (map[int64]domain.TaskExecution, error) {
	daoExecutions, err := r.dao.FindExecutionByPlanID(ctx, planExecID)
	if err != nil {
		return nil, err
	}

	// 将DAO模型转换为领域模型
	result := make(map[int64]domain.TaskExecution)
	for taskID := range daoExecutions {
		daoExecution := daoExecutions[taskID]
		result[taskID] = r.toDomain(daoExecution)
	}

	return result, nil
}

func (r *taskExecutionRepository) Create(ctx context.Context, execution domain.TaskExecution) (domain.TaskExecution, error) {
	// 验证必填字段
	if execution.Task.ID == 0 {
		return domain.TaskExecution{}, errors.New("Task.ID不能为空")
	}
	// 获取完整的Task信息
	completeTask, err := r.taskRepo.GetByID(ctx, execution.Task.ID)
	if err != nil {
		return domain.TaskExecution{}, fmt.Errorf("获取Task信息失败: %w", err)
	}
	// 创建TaskExecution

	execution.Task = completeTask
	created, err := r.dao.Create(ctx, r.toEntity(execution))
	if err != nil {
		return domain.TaskExecution{}, err
	}
	return r.toDomain(created), nil
}

func (r *taskExecutionRepository) UpdateStatus(ctx context.Context, id int64, status domain.TaskExecutionStatus) error {
	return r.dao.UpdateStatus(ctx, id, status.String())
}

func (r *taskExecutionRepository) GetByID(ctx context.Context, id int64) (domain.TaskExecution, error) {
	daoExecution, err := r.dao.GetByID(ctx, id)
	if err != nil {
		return domain.TaskExecution{}, err
	}
	return r.toDomain(daoExecution), nil
}

func (r *taskExecutionRepository) FindRetryableExecutions(ctx context.Context, limit int) ([]domain.TaskExecution, error) {
	daoExecutions, err := r.dao.FindRetryableExecutions(ctx, limit)
	if err != nil {
		return nil, err
	}
	return slice.Map(daoExecutions, func(_ int, src dao.TaskExecution) domain.TaskExecution {
		return r.toDomain(src)
	}), nil
}

func (r *taskExecutionRepository) UpdateRetryResult(ctx context.Context, id, retryCount, nextRetryTime int64, status domain.TaskExecutionStatus, progress int32, endTime int64, scheduleParams map[string]string, executorNodeID string) error {
	return r.dao.UpdateRetryResult(ctx, id, retryCount, nextRetryTime, status.String(), progress, endTime, scheduleParams, executorNodeID)
}

func (r *taskExecutionRepository) SetRunningState(ctx context.Context, id int64, progress int32, executorNodeID string) error {
	return r.dao.SetRunningState(ctx, id, progress, executorNodeID)
}

func (r *taskExecutionRepository) UpdateRunningProgress(ctx context.Context, id int64, progress int32) error {
	return r.dao.UpdateProgress(ctx, id, progress)
}

func (r *taskExecutionRepository) UpdateScheduleResult(ctx context.Context, id int64, status domain.TaskExecutionStatus, progress int32, endTime int64, scheduleParams map[string]string, executorNodeID string) error {
	return r.dao.UpdateScheduleResult(ctx, id, status.String(), progress, endTime, scheduleParams, executorNodeID)
}

func (r *taskExecutionRepository) FindReschedulableExecutions(ctx context.Context, limit int) ([]domain.TaskExecution, error) {
	daoExecutions, err := r.dao.FindReschedulableExecutions(ctx, limit)
	if err != nil {
		return nil, err
	}
	return slice.Map(daoExecutions, func(_ int, src dao.TaskExecution) domain.TaskExecution {
		return r.toDomain(src)
	}), nil
}

func (r *taskExecutionRepository) FindTimeoutExecutions(ctx context.Context, limit int) ([]domain.TaskExecution, error) {
	daoExecutions, err := r.dao.FindTimeoutExecutions(ctx, limit)
	if err != nil {
		return nil, err
	}
	return slice.Map(daoExecutions, func(_ int, src dao.TaskExecution) domain.TaskExecution {
		return r.toDomain(src)
	}), nil
}

// toEntity 将领域模型转换为DAO模型
func (r *taskExecutionRepository) toEntity(execution domain.TaskExecution) dao.TaskExecution {
	var grpcConfig sqlx.JSONColumn[domain.GrpcConfig]
	if execution.Task.GrpcConfig != nil {
		grpcConfig = sqlx.JSONColumn[domain.GrpcConfig]{Val: *execution.Task.GrpcConfig, Valid: true}
	}

	var httpConfig sqlx.JSONColumn[domain.HTTPConfig]
	if execution.Task.HTTPConfig != nil {
		httpConfig = sqlx.JSONColumn[domain.HTTPConfig]{Val: *execution.Task.HTTPConfig, Valid: true}
	}

	var retryConfig sqlx.JSONColumn[domain.RetryConfig]
	if execution.Task.RetryConfig != nil {
		retryConfig = sqlx.JSONColumn[domain.RetryConfig]{Val: *execution.Task.RetryConfig, Valid: true}
	}

	var taskScheduleParams sqlx.JSONColumn[map[string]string]
	if execution.Task.ScheduleParams != nil {
		taskScheduleParams = sqlx.JSONColumn[map[string]string]{Val: execution.Task.ScheduleParams, Valid: true}
	}

	var executorNodeID sql.NullString
	if execution.ExecutorNodeID != "" {
		executorNodeID = sql.NullString{String: execution.ExecutorNodeID, Valid: true}
	}

	return dao.TaskExecution{
		ID: execution.ID,
		// 从Task展开的冗余字段
		TaskID:                  execution.Task.ID,
		TaskName:                execution.Task.Name,
		TaskCronExpr:            execution.Task.CronExpr,
		TaskGrpcConfig:          grpcConfig,
		TaskHTTPConfig:          httpConfig,
		TaskRetryConfig:         retryConfig,
		TaskMaxExecutionSeconds: execution.Task.MaxExecutionSeconds,
		TaskVersion:             execution.Task.Version,
		TaskScheduleNodeID:      execution.Task.ScheduleNodeID,
		TaskScheduleParams:      taskScheduleParams,
		// TaskExecution自身字段
		Deadline:        execution.Deadline,
		ExecutorNodeID:  executorNodeID,
		Stime:           execution.StartTime,
		Etime:           execution.EndTime,
		RetryCount:      execution.RetryCount,
		NextRetryTime:   execution.NextRetryTime,
		RunningProgress: execution.RunningProgress,
		Status:          execution.Status.String(),
		Ctime:           execution.CTime,
		Utime:           execution.UTime,
	}
}

// toDomain 将DAO模型转换为领域模型
func (r *taskExecutionRepository) toDomain(daoExecution dao.TaskExecution) domain.TaskExecution {
	var taskGrpcConfig *domain.GrpcConfig
	if daoExecution.TaskGrpcConfig.Valid {
		taskGrpcConfig = &daoExecution.TaskGrpcConfig.Val
	}

	var taskHTTPConfig *domain.HTTPConfig
	if daoExecution.TaskHTTPConfig.Valid {
		taskHTTPConfig = &daoExecution.TaskHTTPConfig.Val
	}

	var taskRetryConfig *domain.RetryConfig
	if daoExecution.TaskRetryConfig.Valid {
		taskRetryConfig = &daoExecution.TaskRetryConfig.Val
	}

	var taskScheduleParams map[string]string
	if daoExecution.TaskScheduleParams.Valid {
		taskScheduleParams = daoExecution.TaskScheduleParams.Val
	}

	var executorNodeID string
	if daoExecution.ExecutorNodeID.Valid {
		executorNodeID = daoExecution.ExecutorNodeID.String
	}

	return domain.TaskExecution{
		ID: daoExecution.ID,
		Task: domain.Task{
			ID:                  daoExecution.TaskID,
			Name:                daoExecution.TaskName,
			CronExpr:            daoExecution.TaskCronExpr,
			GrpcConfig:          taskGrpcConfig,
			HTTPConfig:          taskHTTPConfig,
			RetryConfig:         taskRetryConfig,
			MaxExecutionSeconds: daoExecution.TaskMaxExecutionSeconds,
			ScheduleParams:      taskScheduleParams,
			ScheduleNodeID:      daoExecution.TaskScheduleNodeID,
			Version:             daoExecution.TaskVersion,
		},

		Deadline:        daoExecution.Deadline,
		ExecutorNodeID:  executorNodeID,
		StartTime:       daoExecution.Stime,
		EndTime:         daoExecution.Etime,
		RetryCount:      daoExecution.RetryCount,
		NextRetryTime:   daoExecution.NextRetryTime,
		RunningProgress: daoExecution.RunningProgress,
		Status:          domain.TaskExecutionStatus(daoExecution.Status),
		CTime:           daoExecution.Ctime,
		UTime:           daoExecution.Utime,
	}
}
