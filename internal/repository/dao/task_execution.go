package dao

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Duke1616/ecmdb/internal/domain"
	"github.com/Duke1616/ecmdb/internal/errs"
	"github.com/Duke1616/ecmdb/pkg/sqlx"
	"gorm.io/gorm"
)

const (
	TaskExecutionStatusPrepare           = "PREPARE"
	TaskExecutionStatusRunning           = "RUNNING"
	TaskExecutionStatusFailedRetryable   = "FAILED_RETRYABLE"
	TaskExecutionStatusFailedRescheduled = "FAILED_RESCHEDULED"

	milliseconds = 1000
)

// TaskExecution 任务执行记录表DAO对象
type TaskExecution struct {
	ID int64 `gorm:"type:bigint;primaryKey;autoIncrement;"`
	// 下面都是创建当前 TaskExecution 时从对应的Task直接拷贝过来的冗余信息
	TaskID                  int64                               `gorm:"type:bigint;not null;comment:'任务ID'"`
	TaskName                string                              `gorm:"type:varchar(255);not null;comment:'任务名称'"`
	TaskCronExpr            string                              `gorm:"type:varchar(100);not null;comment:'cron表达式'"`
	TaskGrpcConfig          sqlx.JSONColumn[domain.GrpcConfig]  `gorm:"type:json;comment:'gRPC配置：{\"serviceName\": \"user-service\"}'"`
	TaskHTTPConfig          sqlx.JSONColumn[domain.HTTPConfig]  `gorm:"type:json;comment:'HTTP配置：{\"endpoint\": \"https://host:port/api\"}'"`
	TaskRetryConfig         sqlx.JSONColumn[domain.RetryConfig] `gorm:"type:json;comment:'重试配置'"`
	TaskMaxExecutionSeconds int64                               `gorm:"type:bigint;not null;default:86400;comment:'最大执行秒数，默认24小时'"`
	TaskVersion             int64                               `gorm:"type:bigint;not null;comment:'创建时Task的版本号'"`
	TaskScheduleNodeID      string                              `gorm:"type:varchar(255);not null;comment:'创建此执行的调度节点ID'"`
	TaskScheduleParams      sqlx.JSONColumn[map[string]string]  `gorm:"type:json;comment:'创建时Task的调度参数快照'"`
	TaskPlanExecID          int64                               `gorm:"type:bigint;not null;comment:'对应Plan的执行计划'"`
	TaskPlanID              int64                               `gorm:"type:bigint;not null;comment:'对应Plan的ID'"`

	// 下面这些是 TaskExecution 的自身信息
	ExecutorNodeID  sql.NullString `gorm:"type:varchar(255);comment:'执行节点的 nodeID，用于记录是哪个节点处理了任务'"`
	Deadline        int64          `gorm:"type:bigint;not null;comment:'任务执行截止时间（毫秒时间戳）'"`
	Stime           int64          `gorm:"type:bigint;comment:'开始时间'"`
	Etime           int64          `gorm:"type:bigint;comment:'结束时间'"`
	RetryCount      int64          `gorm:"type:bigint;not null;default:0;comment:'已重试次数'"`
	NextRetryTime   int64          `gorm:"type:bigint;comment:'下次重试时间'"`
	RunningProgress int32          `gorm:"type:int;default:0;comment:'执行进度0-100，RUNNING状态下有效'"`
	Status          string         `gorm:"type:ENUM('PREPARE', 'RUNNING', 'FAILED_RETRYABLE', 'FAILED_RESCHEDULED', 'FAILED', 'SUCCESS');not null;default:'PREPARE';comment:'执行状态: PREPARE-初始化(没有执行节点在执行）, RUNNING-执行中（有执行节点在执行）, FAILED_RETRYABLE-可重试失败, FAILED_RESCHEDULED-重调度失败， FAILED-失败, SUCCESS-成功'"`
	Ctime           int64          `gorm:"comment:'创建时间'"`
	Utime           int64          `gorm:"comment:'更新时间'"`
}

// TableName 指定表名
func (TaskExecution) TableName() string {
	return "task_executions"
}

type TaskExecutionDAO interface {
	// Create 创建任务执行记录
	Create(ctx context.Context, execution TaskExecution) (TaskExecution, error)
	// BatchCreate 批量创建执行记录
	BatchCreate(ctx context.Context, executions []TaskExecution) ([]TaskExecution, error)
	// GetByID 根据ID获取执行记录
	GetByID(ctx context.Context, id int64) (TaskExecution, error)
	// UpdateStatus 更新执行状态
	UpdateStatus(ctx context.Context, id int64, status string) error
	// FindRetryableExecutions 查找所有可以重试的执行记录
	// limit: 查询结果数量限制
	FindRetryableExecutions(ctx context.Context, limit int) ([]TaskExecution, error)
	// UpdateRetryResult 更新重试结果
	UpdateRetryResult(ctx context.Context, id, retryCount, nextRetryTime int64, status string, progress int32, endTime int64, scheduleParams map[string]string, executorNodeID string) error
	// SetRunningState 设置任务为运行状态并更新进度
	SetRunningState(ctx context.Context, id int64, progress int32, executorNodeID string) error
	// UpdateProgress 更新任务执行进度、开始时间（仅在RUNNING状态下有效）
	UpdateProgress(ctx context.Context, id int64, progress int32) error
	// UpdateScheduleResult 更新调度结果
	UpdateScheduleResult(ctx context.Context, id int64, status string, progress int32, endTime int64, scheduleParams map[string]string, executorNodeID string) error
	// FindReschedulableExecutions 查找所有可以重调度的执行记录
	FindReschedulableExecutions(ctx context.Context, limit int) ([]TaskExecution, error)
	// FindExecutionByPlanID 查找对应planExecID下的所有执行计划
	FindExecutionByPlanID(ctx context.Context, planExecID int64) (map[int64]TaskExecution, error)
	FindByTaskID(ctx context.Context, taskID int64) ([]TaskExecution, error)
	FindExecutionByTaskIDAndPlanExecID(ctx context.Context, taskID int64, planExecID int64) (TaskExecution, error)
	// FindTimeoutExecutions 查找超时的执行记录
	FindTimeoutExecutions(ctx context.Context, limit int) ([]TaskExecution, error)
}

type GORMTaskExecutionDAO struct {
	db *gorm.DB
}

func (g *GORMTaskExecutionDAO) FindExecutionByTaskIDAndPlanExecID(ctx context.Context, taskID, planExecID int64) (TaskExecution, error) {
	var exec TaskExecution
	err := g.db.WithContext(ctx).Where("task_id = ? AND plan_exec_id = ? ", taskID, planExecID).Order("ctime DESC").First(&exec).Error
	if err != nil {
		return TaskExecution{}, fmt.Errorf("查询任务 %d 的执行记录失败: %w", taskID, err)
	}
	return exec, nil
}

func (g *GORMTaskExecutionDAO) FindByTaskID(ctx context.Context, taskID int64) ([]TaskExecution, error) {
	var executions []TaskExecution
	err := g.db.WithContext(ctx).Where("task_id = ?", taskID).Order("ctime DESC").Find(&executions).Error
	if err != nil {
		return nil, fmt.Errorf("查询任务 %d 的执行记录失败: %w", taskID, err)
	}
	return executions, nil
}

func (g *GORMTaskExecutionDAO) FindExecutionByPlanID(ctx context.Context, planExecID int64) (map[int64]TaskExecution, error) {
	var executions []TaskExecution
	err := g.db.WithContext(ctx).
		Where("task_plan_exec_id = ? ", planExecID).
		Order("ctime DESC").
		Find(&executions).Error
	if err != nil {
		return nil, err
	}

	result := make(map[int64]TaskExecution)
	for idx := range executions {
		execution := executions[idx]
		result[execution.TaskID] = execution
	}

	return result, nil
}

func NewGORMTaskExecutionDAO(db *gorm.DB) TaskExecutionDAO {
	return &GORMTaskExecutionDAO{db: db}
}

func (g *GORMTaskExecutionDAO) Create(ctx context.Context, execution TaskExecution) (TaskExecution, error) {
	now := time.Now().UnixMilli()
	execution.Utime, execution.Ctime = now, now
	// 计算deadline
	execution.Deadline = now + execution.TaskMaxExecutionSeconds*milliseconds

	// GORM的Create会自动填充ID到结构体中
	err := g.db.WithContext(ctx).Create(&execution).Error
	if err != nil {
		return TaskExecution{}, fmt.Errorf("创建执行记录失败: %w", err)
	}

	// 返回包含生成ID的实体
	return execution, nil
}

func (g *GORMTaskExecutionDAO) BatchCreate(ctx context.Context, executions []TaskExecution) ([]TaskExecution, error) {
	now := time.Now().UnixMilli()
	for i := range executions {
		executions[i].Ctime, executions[i].Utime = now, now
	}
	err := g.db.WithContext(ctx).CreateInBatches(executions, len(executions)).Error
	if err != nil {
		return nil, fmt.Errorf("创建执行记录失败: %w", err)
	}
	// 返回包含生成ID的实体
	return executions, nil
}

func (g *GORMTaskExecutionDAO) GetByID(ctx context.Context, id int64) (TaskExecution, error) {
	var execution TaskExecution
	err := g.db.WithContext(ctx).Where("id = ?", id).First(&execution).Error
	if err != nil {
		return TaskExecution{}, fmt.Errorf("%w: ID=%d, %w", errs.ErrExecutionNotFound, id, err)
	}
	return execution, err
}

func (g *GORMTaskExecutionDAO) UpdateStatus(ctx context.Context, id int64, status string) error {
	result := g.db.WithContext(ctx).
		Model(&TaskExecution{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status": status,
			"utime":  time.Now().UnixMilli(),
		})
	if result.Error != nil {
		return fmt.Errorf("%w: 数据库操作失败: %w", errs.ErrUpdateExecutionStatusFailed, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("%w：ID=%d", errs.ErrUpdateExecutionStatusFailed, id)
	}
	return nil
}

func (g *GORMTaskExecutionDAO) FindRetryableExecutions(ctx context.Context, limit int) ([]TaskExecution, error) {
	var executions []TaskExecution
	now := time.Now().UnixMilli()

	// 复杂查询：查找可重试的执行记录
	err := g.db.WithContext(ctx).
		// 过滤掉已达最大重试次数的记录
		// FAILED_RETRYABLE状态 - 执行失败但可重试
		Where(`status=? AND next_retry_time <= ?`, TaskExecutionStatusFailedRetryable, now).
		// 确保到了可以执行的时间
		Where(" next_retry_time <= ?", now).
		Limit(limit).
		Find(&executions).Error
	return executions, err
}

func (g *GORMTaskExecutionDAO) UpdateRetryResult(ctx context.Context, id, retryCount, nextRetryTime int64, status string, progress int32, endTime int64, scheduleParams map[string]string, executorNodeID string) error {
	result := g.db.WithContext(ctx).
		Model(&TaskExecution{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"retry_count":          retryCount,
			"next_retry_time":      nextRetryTime,
			"status":               status,
			"running_progress":     progress,
			"etime":                endTime,
			"task_schedule_params": scheduleParams,
			"executor_node_id":     sql.NullString{String: executorNodeID, Valid: executorNodeID != ""},
			"utime":                time.Now().UnixMilli(),
		})

	if result.Error != nil {
		return fmt.Errorf("%w: 数据库操作失败: %w", errs.ErrUpdateExecutionRetryResultFailed, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("%w: ID=%d", errs.ErrUpdateExecutionRetryResultFailed, id)
	}
	return nil
}

func (g *GORMTaskExecutionDAO) SetRunningState(ctx context.Context, id int64, progress int32, executorNodeID string) error {
	now := time.Now().UnixMilli()

	// 首先查询任务执行记录
	var execution TaskExecution
	err := g.db.WithContext(ctx).Where("id = ?", id).First(&execution).Error
	if err != nil {
		return fmt.Errorf("%w: 查询执行记录失败: %w", errs.ErrSetExecutionStateRunningFailed, err)
	}

	// 重新计算deadline
	newDeadline := now + execution.TaskMaxExecutionSeconds*milliseconds

	result := g.db.WithContext(ctx).
		Model(&TaskExecution{}).
		Where("id = ? AND (status = ? OR status = ? OR status = ?) ",
			id, TaskExecutionStatusPrepare, TaskExecutionStatusFailedRetryable, TaskExecutionStatusFailedRescheduled).
		Updates(map[string]any{
			"status":           TaskExecutionStatusRunning,
			"running_progress": progress,
			"stime":            now,
			"deadline":         newDeadline,
			"utime":            now,
			"executor_node_id": sql.NullString{String: executorNodeID, Valid: executorNodeID != ""},
		})

	if result.Error != nil {
		return fmt.Errorf("%w: 数据库操作失败: %w", errs.ErrSetExecutionStateRunningFailed, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("%w: 任务不在PREPARE/FAILED_RETRYABLE状态或不存在, ID=%d", errs.ErrSetExecutionStateRunningFailed, id)
	}
	return nil
}

func (g *GORMTaskExecutionDAO) UpdateProgress(ctx context.Context, id int64, progress int32) error {
	result := g.db.WithContext(ctx).
		Model(&TaskExecution{}).
		Where("id = ? AND status = ?", id, TaskExecutionStatusRunning).
		Updates(map[string]any{
			"running_progress": progress,
			"utime":            time.Now().UnixMilli(),
		})

	if result.Error != nil {
		return fmt.Errorf("%w: 数据库操作失败: %w", errs.ErrUpdateExecutionRunningProgressFailed, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("%w: 任务不在RUNNING状态或不存在，ID=%d", errs.ErrUpdateExecutionRunningProgressFailed, id)
	}
	return nil
}

func (g *GORMTaskExecutionDAO) UpdateScheduleResult(ctx context.Context, id int64, status string, progress int32, endTime int64, scheduleParams map[string]string, executorNodeID string) error {
	result := g.db.WithContext(ctx).
		Model(&TaskExecution{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":               status,
			"running_progress":     progress,
			"etime":                endTime,
			"task_schedule_params": sqlx.JSONColumn[map[string]string]{Val: scheduleParams, Valid: scheduleParams != nil},
			"executor_node_id":     sql.NullString{String: executorNodeID, Valid: executorNodeID != ""},
			"utime":                time.Now().UnixMilli(),
		})
	if result.Error != nil {
		return fmt.Errorf("%w: 数据库操作失败: %w", errs.ErrUpdateExecutionStatusAndEndTimeFailed, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("%w: ID=%d", errs.ErrUpdateExecutionStatusAndEndTimeFailed, id)
	}
	return nil
}

func (g *GORMTaskExecutionDAO) FindReschedulableExecutions(ctx context.Context, limit int) ([]TaskExecution, error) {
	var executions []TaskExecution
	// 查找可重调度的执行记录
	err := g.db.WithContext(ctx).
		Where("status = ? AND (sharding_parent_id IS NULL OR sharding_parent_id > 0) ", TaskExecutionStatusFailedRescheduled).
		Order("utime ASC").
		Limit(limit).
		Find(&executions).Error
	return executions, err
}

func (g *GORMTaskExecutionDAO) FindTimeoutExecutions(ctx context.Context, limit int) ([]TaskExecution, error) {
	var executions []TaskExecution
	now := time.Now().UnixMilli()

	err := g.db.WithContext(ctx).
		Where("deadline <= ? AND status = ?", now, TaskExecutionStatusRunning).
		Order("deadline ASC").
		Limit(limit).
		Find(&executions).Error

	return executions, err
}
