package dao

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/errs"
	"github.com/Duke1616/etask/pkg/sqlx"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	TaskExecutionStatusWaitingPull       = "WAITING_PULL"
	TaskExecutionStatusPrepare           = "PREPARE"
	TaskExecutionStatusRunning           = "RUNNING"
	TaskExecutionStatusFailedRetryable   = "FAILED_RETRYABLE"
	TaskExecutionStatusFailedRescheduled = "FAILED_RESCHEDULED"
	TaskExecutionStatusCancelled         = "CANCELLED"

	milliseconds = 1000
)

// TaskExecution 任务执行记录表DAO对象
type TaskExecution struct {
	ID       int64 `gorm:"type:bigint;primaryKey;autoIncrement;"`
	TenantID int64 `gorm:"type:bigint unsigned;not null;default:0;index;uniqueIndex:uk_execution_request,priority:1;comment:租户ID"`
	// Source 标识执行记录来自正式任务还是 Codebook 试运行。
	Source string `gorm:"type:varchar(32);not null;default:'TASK';index;uniqueIndex:uk_execution_request,priority:2;comment:'执行来源：TASK、CODEBOOK_PREVIEW、WORKFLOW'"`
	// RequestID 仅供外部来源幂等提交使用；NULL 不参与唯一约束。
	RequestID sql.NullString `gorm:"type:varchar(128);uniqueIndex:uk_execution_request,priority:3;comment:'外部幂等请求标识'"`
	// 下面都是创建当前 TaskExecution 时从对应的Task直接拷贝过来的冗余信息
	TaskID                  int64                                        `gorm:"type:bigint;not null;index:idx_execution_task_status,priority:1;comment:'任务ID'"`
	TaskRunnerID            int64                                        `gorm:"type:bigint;not null;default:0;comment:'创建时Task引用的执行单元ID'"`
	TaskName                string                                       `gorm:"type:varchar(255);not null;comment:'任务名称'"`
	TaskType                string                                       `gorm:"type:ENUM('RECURRING', 'ONE_TIME');not null;default:'RECURRING';comment:'任务类型: RECURRING-定时任务(循环执行), ONE_TIME-一次性任务(执行一次后停止)'"`
	TaskCronExpr            string                                       `gorm:"type:varchar(100);not null;comment:'cron表达式'"`
	TaskGrpcConfig          sqlx.JSONColumn[domain.GrpcConfig]           `gorm:"type:json;comment:'gRPC配置：{\"serviceName\": \"user-service\"}'"`
	TaskHTTPConfig          sqlx.JSONColumn[domain.HTTPConfig]           `gorm:"type:json;comment:'HTTP配置：{\"endpoint\": \"https://host:port/api\"}'"`
	TaskRetryConfig         sqlx.JSONColumn[domain.RetryConfig]          `gorm:"type:json;comment:'重试配置'"`
	TaskMaxExecutionSeconds int64                                        `gorm:"type:bigint;not null;default:86400;comment:'最大执行秒数，默认24小时'"`
	TaskVersion             int64                                        `gorm:"type:bigint;not null;comment:'创建时Task的版本号'"`
	TaskScheduleNodeID      string                                       `gorm:"type:varchar(255);not null;comment:'创建此执行的调度节点ID'"`
	TaskScheduleParams      sqlx.JSONColumn[map[string]string]           `gorm:"type:json;comment:'创建时Task的调度参数快照'"`
	TaskParamOverrides      sqlx.JSONColumn[map[string]string]           `gorm:"type:json;comment:'本次手动启动消费的参数覆盖快照'"`
	Artifact                sqlx.JSONColumn[[]domain.ArtifactRef]        `gorm:"type:json;comment:'本次执行固定的代码制品层'"`
	Variables               sqlx.JSONColumn[domain.ExecutionVariableSet] `gorm:"type:json;comment:'本次执行固定的变量快照'"`
	Program                 sqlx.JSONColumn[domain.Program]              `gorm:"type:json;comment:'本次执行固定的程序来源'"`
	ExecutionRoute          sqlx.JSONColumn[domain.ExecutionRoute]       `gorm:"type:json;comment:'本次执行固定的传输和派发路由'"`

	// 下面这些是 TaskExecution 的自身信息
	ExecutorNodeID  sql.NullString `gorm:"type:varchar(255);comment:'执行节点的 nodeID，用于记录是哪个节点处理了任务'"`
	Deadline        int64          `gorm:"type:bigint;not null;index:idx_execution_timeout,priority:2;comment:'任务执行截止时间（毫秒时间戳）'"`
	Stime           int64          `gorm:"type:bigint;comment:'开始时间'"`
	Etime           int64          `gorm:"type:bigint;comment:'结束时间'"`
	RetryCount      int64          `gorm:"type:bigint;not null;default:0;comment:'已重试次数'"`
	NextRetryTime   int64          `gorm:"type:bigint;index:idx_execution_retry,priority:2;comment:'下次重试时间'"`
	RunningProgress int32          `gorm:"type:int;default:0;comment:'执行进度0-100，RUNNING状态下有效'"`
	Status          string         `gorm:"type:ENUM('WAITING_PULL', 'PREPARE', 'RUNNING', 'FAILED_RETRYABLE', 'FAILED_RESCHEDULED', 'FAILED', 'SUCCESS', 'CANCELLED');not null;default:'PREPARE';index:idx_execution_task_status,priority:2;index:idx_execution_retry,priority:1;index:idx_execution_reschedule,priority:1;index:idx_execution_timeout,priority:1;index:idx_execution_pull,priority:1;comment:'执行状态: PREPARE-初始化，RUNNING-执行中，FAILED_RETRYABLE-可重试失败，FAILED_RESCHEDULED-重调度失败，FAILED-失败，SUCCESS-成功，CANCELLED-强制终止'"`
	ExecMode        string         `gorm:"type:ENUM('PUSH', 'PULL');not null;default:'PUSH';comment:'本次执行采用的模式（PUSH-中心推送/PULL-边缘拉取）'"`
	TaskResult      string         `gorm:"type:text;comment:'任务执行的结构化结果（JSON格式）'"`
	Ctime           int64          `gorm:"index:idx_execution_pull,priority:2;comment:'创建时间'"`
	Utime           int64          `gorm:"index:idx_execution_reschedule,priority:2;comment:'更新时间'"`
}

// TableName 指定表名
func (TaskExecution) TableName() string {
	return "task_executions"
}

type TaskExecutionDAO interface {
	// Create 创建任务执行记录
	Create(ctx context.Context, execution TaskExecution) (TaskExecution, error)
	// CreateAndConsumeOverride 创建执行记录并原子消费本次启动参数。
	CreateAndConsumeOverride(ctx context.Context, execution TaskExecution, taskID int64) (TaskExecution, error)
	// BatchCreate 批量创建执行记录
	BatchCreate(ctx context.Context, executions []TaskExecution) ([]TaskExecution, error)
	// GetByID 根据ID获取执行记录
	GetByID(ctx context.Context, id int64) (TaskExecution, error)
	// FindByRequestID 根据执行来源和幂等请求标识查询执行记录。
	FindByRequestID(ctx context.Context, source, requestID string) (TaskExecution, error)
	// UpdateStatus 仅在当前状态符合预期时更新执行状态。
	UpdateStatus(ctx context.Context, id int64, expectedStatuses []string, status string) error
	// FindRetryableExecutions 查找所有可以重试的执行记录
	// limit: 查询结果数量限制
	FindRetryableExecutions(ctx context.Context, limit int) ([]TaskExecution, error)
	// UpdateRetryResult 仅在当前状态符合预期时更新重试结果。
	UpdateRetryResult(ctx context.Context, id, retryCount, nextRetryTime int64, expectedStatus, status string, progress int32, endTime int64, scheduleParams map[string]string, executorNodeID string) error
	// SetRunningState 设置任务为运行状态并更新进度
	SetRunningState(ctx context.Context, id int64, progress int32, executorNodeID string) error
	// UpdateProgress 更新任务执行进度、开始时间（仅在RUNNING状态下有效）
	UpdateProgress(ctx context.Context, id int64, progress int32, executorNodeID string) error
	// UpdateScheduleResult 仅在当前状态符合预期时更新调度结果。
	// 返回 false 表示状态已被其他请求推进，当前请求没有写入。
	UpdateScheduleResult(ctx context.Context, id int64, expectedStatuses []string, status string, progress int32, endTime int64, scheduleParams map[string]string, executorNodeID string, taskResult string) (bool, error)
	// FindReschedulableExecutions 查找所有可以重调度的执行记录
	FindReschedulableExecutions(ctx context.Context, limit int) ([]TaskExecution, error)
	// FindExecutionByPlanID 查找对应planExecID下的所有执行计划
	FindExecutionByPlanID(ctx context.Context, planExecID int64) (map[int64]TaskExecution, error)
	// FindByTaskID 根据任务ID查找执行记录
	FindByTaskID(ctx context.Context, taskID int64) ([]TaskExecution, error)
	// HasNonTerminalByTaskID 判断任务是否存在未结束的执行记录。
	HasNonTerminalByTaskID(ctx context.Context, taskID int64) (bool, error)
	// ListByTaskID 根据任务ID分页查找执行记录
	ListByTaskID(ctx context.Context, taskID int64, offset, limit int) ([]TaskExecution, error)
	// CountByTaskID 根据任务ID统计执行记录总数
	CountByTaskID(ctx context.Context, taskID int64) (int64, error)
	// FindByTaskIDs 批量根据任务ID查找执行记录
	FindByTaskIDs(ctx context.Context, taskIDs []int64) ([]TaskExecution, error)
	// FindExecutionByTaskIDAndPlanExecID 根据任务ID和执行计划ID查找执行记录
	FindExecutionByTaskIDAndPlanExecID(ctx context.Context, taskID int64, planExecID int64) (TaskExecution, error)
	// FindTimeoutExecutions 查找超时的执行记录
	FindTimeoutExecutions(ctx context.Context, limit int) ([]TaskExecution, error)
	// ClaimPullTask 原子抢占一个当前节点支持的等待拉取任务。
	ClaimPullTask(ctx context.Context, serviceName, executorNodeID string,
		handlerNames []string) (TaskExecution, error)
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
	// 显式 Select 展现所需的字段，排除不必要的超大 JSON 配置字段，提升大记录下的读取性能
	err := g.db.WithContext(ctx).
		Select("id", "tenant_id", "task_id", "task_name", "task_type", "task_cron_expr", "task_max_execution_seconds", "task_version", "task_schedule_node_id", "deadline", "executor_node_id", "stime", "etime", "retry_count", "next_retry_time", "running_progress", "status", "exec_mode", "task_result", "ctime", "utime").
		Where("task_id = ?", taskID).
		Order("ctime DESC").
		Find(&executions).Error
	if err != nil {
		return nil, fmt.Errorf("查询任务 %d 的执行记录失败: %w", taskID, err)
	}
	return executions, nil
}

// HasNonTerminalByTaskID 使用带 LIMIT 的存在性查询，避免为防重复调度加载完整执行快照。
func (g *GORMTaskExecutionDAO) HasNonTerminalByTaskID(ctx context.Context, taskID int64) (bool, error) {
	var row struct {
		TaskID int64 `gorm:"column:task_id"`
	}
	result := g.db.WithContext(ctx).
		Model(&TaskExecution{}).
		// 选择组合索引中的字段，避免加载完整执行记录，也避免 SELECT 1 的列名无法映射。
		Select("task_id").
		Where("task_id = ? AND status IN ?", taskID, nonTerminalExecutionStatuses()).
		Limit(1).
		Find(&row)
	return result.RowsAffected > 0, result.Error
}

func nonTerminalExecutionStatuses() []string {
	return []string{
		TaskExecutionStatusWaitingPull,
		TaskExecutionStatusPrepare,
		TaskExecutionStatusRunning,
		TaskExecutionStatusFailedRetryable,
		TaskExecutionStatusFailedRescheduled,
	}
}

func (g *GORMTaskExecutionDAO) ListByTaskID(ctx context.Context, taskID int64, offset, limit int) ([]TaskExecution, error) {
	var executions []TaskExecution
	err := g.db.WithContext(ctx).
		Where("task_id = ?", taskID).
		Order("ctime DESC, id DESC").
		Offset(offset).
		Limit(limit).
		Find(&executions).Error
	if err != nil {
		return nil, fmt.Errorf("分页查询任务 %d 的执行记录失败: %w", taskID, err)
	}
	return executions, nil
}

func (g *GORMTaskExecutionDAO) CountByTaskID(ctx context.Context, taskID int64) (int64, error) {
	var count int64
	err := g.db.WithContext(ctx).
		Model(&TaskExecution{}).
		Where("task_id = ?", taskID).
		Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("统计任务 %d 的执行记录总数失败: %w", taskID, err)
	}
	return count, nil
}

func (g *GORMTaskExecutionDAO) FindByTaskIDs(ctx context.Context, taskIDs []int64) ([]TaskExecution, error) {
	var executions []TaskExecution
	// 批量查询排除大字段 task_result，防止大量历史数据排序和传输导致超时
	err := g.db.WithContext(ctx).
		Select("id", "tenant_id", "task_id", "task_name", "task_type", "task_cron_expr", "task_max_execution_seconds", "task_version", "task_schedule_node_id", "deadline", "executor_node_id", "stime", "etime", "retry_count", "next_retry_time", "running_progress", "status", "exec_mode", "ctime", "utime").
		Where("task_id IN ?", taskIDs).
		Order("ctime DESC").
		Find(&executions).Error
	if err != nil {
		return nil, fmt.Errorf("批量查询任务执行记录失败: %w", err)
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

func (g *GORMTaskExecutionDAO) CreateAndConsumeOverride(ctx context.Context, execution TaskExecution,
	taskID int64) (TaskExecution, error) {
	now := time.Now().UnixMilli()
	execution.Utime, execution.Ctime = now, now
	execution.Deadline = now + execution.TaskMaxExecutionSeconds*milliseconds
	err := g.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&execution).Error; err != nil {
			return err
		}
		result := tx.Where("task_id = ?", taskID).
			Delete(&TaskRunParamOverride{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("启动参数覆盖已被其他执行消费: task_id=%d", taskID)
		}
		return nil
	})
	if err != nil {
		return TaskExecution{}, fmt.Errorf("创建执行记录并消费启动参数失败: %w", err)
	}
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

func (g *GORMTaskExecutionDAO) FindByRequestID(ctx context.Context, source, requestID string) (TaskExecution, error) {
	var execution TaskExecution
	err := g.db.WithContext(ctx).
		Where("source = ? AND request_id = ?", source, requestID).
		First(&execution).Error
	return execution, err
}

func (g *GORMTaskExecutionDAO) UpdateStatus(ctx context.Context, id int64,
	expectedStatuses []string, status string) error {
	if len(expectedStatuses) == 0 {
		return fmt.Errorf("%w: expected statuses are empty", errs.ErrInvalidTaskExecutionStatus)
	}
	result := withExecutionStatusCAS(g.db.WithContext(ctx).Model(&TaskExecution{}), id, expectedStatuses).
		Updates(map[string]any{
			"status": status,
			"utime":  time.Now().UnixMilli(),
		})
	if result.Error != nil {
		return fmt.Errorf("%w: 数据库操作失败: %w", errs.ErrUpdateExecutionStatusFailed, result.Error)
	}
	if result.RowsAffected == 0 {
		return executionStatusConflict(id)
	}
	return nil
}

func (g *GORMTaskExecutionDAO) FindRetryableExecutions(ctx context.Context, limit int) ([]TaskExecution, error) {
	var executions []TaskExecution
	now := time.Now().UnixMilli()

	err := g.db.WithContext(ctx).
		Where("status = ? AND next_retry_time <= ?", TaskExecutionStatusFailedRetryable, now).
		Limit(limit).
		Find(&executions).Error
	return executions, err
}

func (g *GORMTaskExecutionDAO) UpdateRetryResult(ctx context.Context, id, retryCount, nextRetryTime int64,
	expectedStatus, status string, progress int32, endTime int64,
	scheduleParams map[string]string, executorNodeID string) error {
	if expectedStatus == "" {
		return fmt.Errorf("%w: expected status is empty", errs.ErrInvalidTaskExecutionStatus)
	}
	result := withExecutionStatusCAS(g.db.WithContext(ctx).Model(&TaskExecution{}), id, []string{expectedStatus}).
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
		return executionStatusConflict(id)
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

func (g *GORMTaskExecutionDAO) UpdateProgress(ctx context.Context, id int64, progress int32, executorNodeID string) error {
	result := g.db.WithContext(ctx).
		Model(&TaskExecution{}).
		Where("id = ? AND status = ?", id, TaskExecutionStatusRunning).
		Updates(map[string]any{
			"running_progress": progress,
			"executor_node_id": sql.NullString{String: executorNodeID, Valid: executorNodeID != ""},
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

func (g *GORMTaskExecutionDAO) UpdateScheduleResult(ctx context.Context, id int64,
	expectedStatuses []string, status string, progress int32, endTime int64,
	scheduleParams map[string]string, executorNodeID string, taskResult string) (bool, error) {
	if len(expectedStatuses) == 0 {
		return false, fmt.Errorf("%w: expected statuses are empty", errs.ErrInvalidTaskExecutionStatus)
	}
	result := withExecutionStatusCAS(g.db.WithContext(ctx).Model(&TaskExecution{}), id, expectedStatuses).
		Updates(map[string]any{
			"status":               status,
			"running_progress":     progress,
			"etime":                endTime,
			"task_schedule_params": sqlx.JSONColumn[map[string]string]{Val: scheduleParams, Valid: scheduleParams != nil},
			"executor_node_id":     sql.NullString{String: executorNodeID, Valid: executorNodeID != ""},
			"task_result":          taskResult,
			"utime":                time.Now().UnixMilli(),
		})
	if result.Error != nil {
		return false, fmt.Errorf("%w: 数据库操作失败: %w", errs.ErrUpdateExecutionStatusAndEndTimeFailed, result.Error)
	}
	if result.RowsAffected == 0 {
		return false, nil
	}
	return true, nil
}

// withExecutionStatusCAS 将来源状态写入更新条件，保证状态检查和写入在同一条 SQL 中完成。
func withExecutionStatusCAS(db *gorm.DB, id int64, expectedStatuses []string) *gorm.DB {
	return db.Where("id = ? AND status IN ?", id, expectedStatuses)
}

func executionStatusConflict(id int64) error {
	return fmt.Errorf("%w: execution not found or status changed, ID=%d",
		errs.ErrInvalidTaskExecutionStatus, id)
}

func (g *GORMTaskExecutionDAO) FindReschedulableExecutions(ctx context.Context, limit int) ([]TaskExecution, error) {
	var executions []TaskExecution
	// 查找可重调度的执行记录
	err := g.db.WithContext(ctx).
		Where("status = ?", TaskExecutionStatusFailedRescheduled).
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

// ClaimPullTask 原子抢占一个当前节点支持的等待拉取任务。
// 使用两阶段行锁机制：阶段一无锁并发读候选主键，阶段二针对候选主键执行 SELECT FOR UPDATE SKIP LOCKED。
// 既借助数据库原生行锁跳过机制消除惊群与抢占冲突，又杜绝了直接基于非唯一 JSON 条件加锁导致的 Next-Key 间隙锁和死锁隐患。
func (g *GORMTaskExecutionDAO) ClaimPullTask(ctx context.Context, serviceName, executorNodeID string,
	handlerNames []string) (TaskExecution, error) {
	if len(handlerNames) == 0 {
		return TaskExecution{}, errs.ErrExecutionNotFound
	}
	now := time.Now().UnixMilli()

	var claimed TaskExecution
	err := g.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 阶段一：无锁并发只读查询最多 5 个就绪任务候选 ID，绝不阻塞任何外部 INSERT 事务
		var candidateIDs []int64
		err := tx.Model(&TaskExecution{}).
			Where("status = ?", TaskExecutionStatusWaitingPull).
			Where("task_grpc_config->>'$.serviceName' = ?", serviceName).
			Where("task_grpc_config->>'$.handlerName' IN ?", handlerNames).
			Order("ctime ASC").
			Limit(5).
			Pluck("id", &candidateIDs).Error
		if err != nil {
			return err
		}
		if len(candidateIDs) == 0 {
			return errs.ErrExecutionNotFound
		}

		// 阶段二：针对主键执行 FOR UPDATE SKIP LOCKED。走 PRIMARY KEY 唯一索引只加单行 Record 锁，
		// 坚决杜绝 Gap Lock 间隙锁；已被并发事务持锁的任务行会被 MySQL 自动跳过
		err = tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("id IN ?", candidateIDs).
			Where("status = ?", TaskExecutionStatusWaitingPull).
			Order("ctime ASC").
			Take(&claimed).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errs.ErrExecutionNotFound
			}
			return err
		}

		// 锁定即代表抢占成功！更新执行状态为 RUNNING 并随事务快速提交（耗时 < 1ms，瞬时释放锁）
		newDeadline := now + claimed.TaskMaxExecutionSeconds*milliseconds
		return tx.Model(&TaskExecution{}).
			Where("id = ?", claimed.ID).
			Updates(map[string]any{
				"status":           TaskExecutionStatusRunning,
				"executor_node_id": sql.NullString{String: executorNodeID, Valid: executorNodeID != ""},
				"stime":            now,
				"deadline":         newDeadline,
				"utime":            now,
			}).Error
	})

	if err != nil {
		return TaskExecution{}, err
	}

	// 同步内存数据并返回
	claimed.Status = TaskExecutionStatusRunning
	claimed.Stime = now
	claimed.Deadline = now + claimed.TaskMaxExecutionSeconds*milliseconds
	claimed.Utime = now
	claimed.ExecutorNodeID = sql.NullString{String: executorNodeID, Valid: executorNodeID != ""}

	return claimed, nil
}
