package dao

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/errs"
	"github.com/Duke1616/etask/pkg/sqlx"
	"gorm.io/gorm"
)

const (
	StatusActive    = "ACTIVE"
	StatusPreempted = "PREEMPTED"
	StatusInactive  = "INACTIVE"
	StatusCompleted = "COMPLETED"
)

// Task 任务表DAO对象
type Task struct {
	ID                    int64                               `gorm:"type:bigint;primaryKey;autoIncrement;"`
	TenantID              int64                               `gorm:"type:bigint unsigned;not null;default:0;index;uniqueIndex:uniq_idx_name_tenant,priority:1;comment:'租户ID'"`
	BizID                 int64                               `gorm:"type:bigint unsigned;not null;default:0;comment:biz_id"`
	BizKey                string                              `gorm:"type:varchar(255);not null;default:'';index:idx_biz;comment:'业务方唯一标识，如工单号'"`
	Name                  string                              `gorm:"type:varchar(255);not null;uniqueIndex:uniq_idx_name_tenant,priority:2;comment:'任务名称'"`
	RunnerID              sql.NullInt64                       `gorm:"column:runner_id;type:bigint;index;comment:'引用的执行单元ID'"`
	Type                  string                              `gorm:"type:ENUM('RECURRING', 'ONE_TIME');not null;default:'RECURRING';comment:'任务类型: RECURRING-定时任务(循环执行), ONE_TIME-一次性任务(执行一次后停止)'"`
	CronExpr              string                              `gorm:"type:varchar(100);not null;comment:'cron表达式'"`
	GrpcConfig            sqlx.JSONColumn[domain.GrpcConfig]  `gorm:"type:json;comment:'gRPC配置：{\"serviceName\": \"user-service\"}'"`
	Program               sqlx.JSONColumn[domain.ProgramSpec] `gorm:"type:json;comment:'用户声明的程序来源'"`
	HTTPConfig            sqlx.JSONColumn[domain.HTTPConfig]  `gorm:"type:json;comment:'HTTP配置：{\"endpoint\": \"https://host:port/api\"}'"`
	RetryConfig           sqlx.JSONColumn[domain.RetryConfig] `gorm:"type:json;comment:'重试配置'"`
	ScheduleParams        sqlx.JSONColumn[map[string]string]  `gorm:"type:json;comment:'每次执行要用到的基础调度参数'"`
	MaxExecutionSeconds   int64                               `gorm:"type:bigint;not null;default:86400;comment:'最大执行秒数，默认24小时'"`
	ScheduleNodeID        sql.NullString                      `gorm:"type:varchar(255);index:idx_schedule_node_id_status,priority:1;comment:'当前抢占的调度节点ID'"`
	NextTime              int64                               `gorm:"type:bigint;not null;index:idx_next_time_status_utime,priority:1;comment:'下次执行时间'"`
	Status                string                              `gorm:"type:ENUM('ACTIVE', 'PREEMPTED', 'INACTIVE', 'COMPLETED');not null;default:'ACTIVE';index:idx_next_time_status_utime,priority:2;index:idx_schedule_node_id_status,priority:2;comment:'任务状态: ACTIVE-可调度, PREEMPTED-已抢占, INACTIVE-停止执行, COMPLETED-已完成。'"`
	Version               int64                               `gorm:"type:bigint;not null;default:1;comment:'版本号，用于乐观锁'"`
	Ctime                 int64                               `gorm:"comment:'创建时间'"`
	Utime                 int64                               `gorm:"index:idx_next_time_status_utime,priority:3;comment:'更新时间'"`
	ExecMode              string                              `gorm:"type:ENUM('PUSH', 'PULL');not null;default:'PUSH';comment:'本次调度采用的执行模式，由 scheduler 选节点时写入'"`
	Metadata              sqlx.JSONColumn[map[string]string]  `gorm:"type:json;comment:'任务参数元数据'"`
	PendingParamOverrides sqlx.JSONColumn[map[string]string]  `gorm:"-"`
	ParamOverrideRules    []TaskParamOverrideRule             `gorm:"-"`
	NotificationRules     []TaskExecutionNotificationRule     `gorm:"-"`
}

// TableName 指定表名
func (Task) TableName() string {
	return "tasks"
}

type TaskDAO interface {
	// Create 创建任务
	Create(ctx context.Context, task Task) (*Task, error)
	// GetByID 根据ID获取任务
	GetByID(ctx context.Context, id int64) (*Task, error)
	// GetByName 根据名称获取任务
	GetByName(ctx context.Context, name string) (*Task, error)
	// FindByPlanID 根据计划ID获取所有子任务
	FindByPlanID(ctx context.Context, planID int64) ([]*Task, error)
	// FindScheduleTasks 查询可调度的任务列表
	// preemptedTimeoutMs: PREEMPTED状态任务的超时时间（毫秒），超过此时间未续约的任务可被重新抢占
	FindScheduleTasks(ctx context.Context, preemptedTimeoutMs int64, limit int) ([]*Task, error)
	// Acquire 抢占任务
	Acquire(ctx context.Context, id, version int64, scheduleNodeID string) (*Task, error)
	// Renew 续约所有被抢占的任务任务
	Renew(ctx context.Context, scheduleNodeID string) error
	// Release 释放任务，更新状态为ACTIVE
	Release(ctx context.Context, id int64, scheduleNodeID string) (*Task, error)
	// UpdateNextTime 更新下一次执行时间
	UpdateNextTime(ctx context.Context, id, version, nextTime int64) (*Task, error)
	// UpdateScheduleParams 更新调度参数（CAS操作）
	UpdateScheduleParams(ctx context.Context, id, version int64, scheduleParams map[string]string) (*Task, error)
	// UpdateStatus 更新任务状态
	UpdateStatus(ctx context.Context, id int64, status string) (*Task, error)
	// Start 启动任务，并保存仅供下一次执行消费的参数覆盖。
	Start(ctx context.Context, id int64, paramOverrides map[string]string) error
	// ResetSchedule 重置一次性任务的调度信息，并保存仅供该次调度消费的参数覆盖。
	ResetSchedule(ctx context.Context, task Task, paramOverrides map[string]string) error
	// UpdateExecMode 更新执行模式快照（由调度器在选定 executor 节点后写入）
	UpdateExecMode(ctx context.Context, id int64, mode string) error
	// Retry 手动重试任务（针对一次性任务，将其状态重置为 ACTIVE 并设置下一次执行时间）
	Retry(ctx context.Context, id, version, nextTime int64) (*Task, error)
	// List 分页获取任务列表
	List(ctx context.Context, bizID int64, offset, limit int) ([]*Task, error)
	// Count 获取任务总数
	Count(ctx context.Context, bizID int64) (int64, error)
	// Update 更新任务配置
	Update(ctx context.Context, task Task) error
	// Delete 删除任务
	Delete(ctx context.Context, id int64) error
}

type GORMTaskDAO struct {
	db *gorm.DB
}

func NewGORMTaskDAO(db *gorm.DB) TaskDAO {
	return &GORMTaskDAO{db: db}
}

func (g *GORMTaskDAO) FindByPlanID(ctx context.Context, planID int64) ([]*Task, error) {
	var tasks []*Task
	err := g.db.WithContext(ctx).Where("plan_id = ?", planID).Find(&tasks).Error
	if err != nil {
		return nil, err
	}
	return tasks, nil
}

func (g *GORMTaskDAO) Create(ctx context.Context, task Task) (*Task, error) {
	now := time.Now().UnixMilli()
	task.Utime, task.Ctime = now, now
	err := g.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&task).Error; err != nil {
			return err
		}
		if err := replaceOverrideRules(tx, task.TenantID, task.ID, task.ParamOverrideRules); err != nil {
			return err
		}
		return replaceExecutionNotificationRules(tx, task.TenantID, task.ID, task.NotificationRules)
	})
	if err != nil {
		if isDuplicateKeyError(err) {
			return nil, fmt.Errorf("%w", errs.ErrTaskNameDuplicate)
		}
		return nil, err
	}
	return &task, nil
}

func (g *GORMTaskDAO) GetByID(ctx context.Context, id int64) (*Task, error) {
	var task Task
	err := g.db.WithContext(ctx).Where("id = ?", id).First(&task).Error
	if err != nil {
		return nil, err
	}
	return &task, nil
}

func (g *GORMTaskDAO) GetByName(ctx context.Context, name string) (*Task, error) {
	var task Task
	err := g.db.WithContext(ctx).Where("name = ?", name).First(&task).Error
	if err != nil {
		return nil, err
	}
	return &task, nil
}

func (g *GORMTaskDAO) FindScheduleTasks(ctx context.Context, preemptedTimeoutMs int64, limit int) ([]*Task, error) {
	var tasks []*Task
	now := time.Now().UnixMilli()
	// 获取所有可调度的任务
	// 1. ACTIVE 状态且到了执行时间的任务
	// 2. PREEMPTED 状态但超时未续约、且没有未终态 execution 的任务（疑似僵尸任务）
	err := g.db.WithContext(ctx).
		Where(`next_time <= ? AND (
			status = ? OR
			(status = ? AND utime <= ? AND NOT EXISTS (
				SELECT 1 FROM task_executions AS execution
				WHERE execution.task_id = tasks.id
				AND execution.status IN ?
			))
		)`, now, StatusActive, StatusPreempted, now-preemptedTimeoutMs,
			nonTerminalExecutionStatuses()).
		Order("next_time ASC").
		Limit(limit).
		Find(&tasks).Error
	if err != nil {
		return nil, err
	}
	return tasks, nil
}

func (g *GORMTaskDAO) Acquire(ctx context.Context, id, version int64, scheduleNodeID string) (*Task, error) {
	var acquiredTask *Task
	err := g.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 在事务中执行更新
		result := tx.Model(&Task{}).
			Where("id = ? AND version = ?", id, version).
			Updates(map[string]any{
				"status":           StatusPreempted,
				"schedule_node_id": scheduleNodeID,
				"version":          gorm.Expr("version + 1"),
				"utime":            time.Now().UnixMilli(),
			})
		if result.Error != nil {
			return result.Error // 事务将自动回滚
		}
		if result.RowsAffected == 0 {
			// 可能是任务已被其他节点抢占，或者任务状态已被修改（导致version变化）
			// 无论哪种情况，都意味着本次抢占失败。
			return errs.ErrTaskPreemptFailed
		}

		// 再次查询，以获取包括新的 version 和 utime 在内的完整任务信息
		var task Task
		if err := tx.Where("id = ?", id).First(&task).Error; err != nil {
			return err
		}
		acquiredTask = &task
		return nil // 提交事务
	})
	if err != nil {
		return nil, err
	}
	return acquiredTask, nil
}

func (g *GORMTaskDAO) Renew(ctx context.Context, scheduleNodeID string) error {
	result := g.db.WithContext(ctx).
		Model(&Task{}).
		Where("schedule_node_id = ? AND status = ?", scheduleNodeID, StatusPreempted).
		Updates(map[string]any{
			"version": gorm.Expr("version + 1"),
			"utime":   time.Now().UnixMilli(),
		})
	if result.Error != nil {
		return fmt.Errorf("%w: 批量续约数据库操作失败: %w", errs.ErrTaskRenewFailed, result.Error)
	}
	return nil
}

func (g *GORMTaskDAO) Release(ctx context.Context, id int64, scheduleNodeID string) (*Task, error) {
	var releasedTask *Task
	err := g.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&Task{}).
			Where("id = ? AND status = ? AND schedule_node_id = ?", id, StatusPreempted, scheduleNodeID).
			Updates(map[string]any{
				"status":           StatusActive,
				"schedule_node_id": gorm.Expr("NULL"),
				"version":          gorm.Expr("version + 1"),
				"utime":            time.Now().UnixMilli(),
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errs.ErrTaskReleaseFailed
		}

		var task Task
		if err := tx.Where("id = ?", id).First(&task).Error; err != nil {
			return err
		}
		releasedTask = &task
		return nil
	})
	if err != nil {
		return nil, err
	}
	return releasedTask, nil
}

func (g *GORMTaskDAO) UpdateNextTime(ctx context.Context, id, version, nextTime int64) (*Task, error) {
	var updatedTask *Task
	err := g.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&Task{}).
			Where("id = ? AND version = ?", id, version).
			Updates(map[string]any{
				"next_time": nextTime,
				"version":   gorm.Expr("version + 1"),
				"utime":     time.Now().UnixMilli(),
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errs.ErrTaskUpdateNextTimeFailed
		}
		var task Task
		if err := tx.Where("id = ?", id).First(&task).Error; err != nil {
			return err
		}
		updatedTask = &task
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updatedTask, nil
}

func (g *GORMTaskDAO) UpdateScheduleParams(ctx context.Context, id, version int64, scheduleParams map[string]string) (*Task, error) {
	var updatedTask *Task
	err := g.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&Task{}).
			Where("id = ? AND version = ?", id, version).
			Updates(map[string]any{
				"schedule_params": sqlx.JSONColumn[map[string]string]{Val: scheduleParams, Valid: scheduleParams != nil},
				"version":         gorm.Expr("version + 1"),
				"utime":           time.Now().UnixMilli(),
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errs.ErrTaskUpdateScheduleParamsFailed
		}
		var task Task
		if err := tx.Where("id = ?", id).First(&task).Error; err != nil {
			return err
		}
		updatedTask = &task
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updatedTask, nil
}

// UpdateStatus 更新任务状态
func (g *GORMTaskDAO) UpdateStatus(ctx context.Context, id int64, status string) (*Task, error) {
	var updatedTask *Task
	err := g.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&Task{}).
			Where("id = ?", id).
			Updates(map[string]any{
				"status":  status,
				"version": gorm.Expr("version + 1"),
				"utime":   time.Now().UnixMilli(),
			})

		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errs.ErrTaskUpdateStatusFailed
		}

		var task Task
		if err := tx.Where("id = ?", id).First(&task).Error; err != nil {
			return err
		}
		updatedTask = &task
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updatedTask, nil
}

func (g *GORMTaskDAO) Start(ctx context.Context, id int64,
	paramOverrides map[string]string) error {
	err := g.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&Task{}).
			Where("id = ? AND status <> ?", id, StatusPreempted).
			Updates(map[string]any{
				"status":           StatusActive,
				"next_time":        time.Now().UnixMilli(),
				"schedule_node_id": gorm.Expr("NULL"),
				"version":          gorm.Expr("version + 1"),
				"utime":            time.Now().UnixMilli(),
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("任务正在运行中，请等待本次执行结束")
		}
		if err := tx.Where("task_id = ?", id).Delete(&TaskRunParamOverride{}).Error; err != nil {
			return err
		}
		if len(paramOverrides) > 0 {
			pending := TaskRunParamOverride{
				TaskID:    id,
				Overrides: sqlx.JSONColumn[map[string]string]{Val: paramOverrides, Valid: true},
			}
			if err := tx.Create(&pending).Error; err != nil {
				return err
			}
		}
		return nil
	})
	return err
}

// ResetSchedule 原子更新调度信息和待消费参数，避免定时任务使用到不完整的启动配置。
func (g *GORMTaskDAO) ResetSchedule(ctx context.Context, task Task,
	paramOverrides map[string]string) error {
	return g.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&Task{}).
			Where("id = ? AND status <> ?", task.ID, StatusPreempted).
			Updates(map[string]any{
				"cron_expr":        task.CronExpr,
				"status":           task.Status,
				"next_time":        task.NextTime,
				"schedule_node_id": gorm.Expr("NULL"),
				"version":          gorm.Expr("version + 1"),
				"utime":            time.Now().UnixMilli(),
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("任务正在运行中，请等待本次执行结束")
		}
		if err := tx.Where("task_id = ?", task.ID).Delete(&TaskRunParamOverride{}).Error; err != nil {
			return err
		}
		if len(paramOverrides) == 0 {
			return nil
		}
		return tx.Create(&TaskRunParamOverride{
			TaskID:    task.ID,
			Overrides: sqlx.JSONColumn[map[string]string]{Val: paramOverrides, Valid: true},
		}).Error
	})
}

// UpdateExecMode 更新任务的执行模式快照（由调度器在选定 executor 节点后写入）
func (g *GORMTaskDAO) UpdateExecMode(ctx context.Context, id int64, mode string) error {
	result := g.db.WithContext(ctx).
		Model(&Task{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"exec_mode": mode,
			"utime":     time.Now().UnixMilli(),
		})
	if result.Error != nil {
		return fmt.Errorf("更新任务执行模式失败: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("更新任务执行模式失败: 任务不存在或当前租户无权访问，ID=%d", id)
	}
	return nil
}

func (g *GORMTaskDAO) Retry(ctx context.Context, id, version, nextTime int64) (*Task, error) {
	var updatedTask *Task
	err := g.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&Task{}).
			Where("id = ? AND version = ?", id, version).
			Updates(map[string]any{
				"status":           StatusActive,
				"next_time":        nextTime,
				"schedule_node_id": gorm.Expr("NULL"),
				"version":          gorm.Expr("version + 1"),
				"utime":            time.Now().UnixMilli(),
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errs.ErrTaskUpdateNextTimeFailed
		}
		var task Task
		if err := tx.Where("id = ?", id).First(&task).Error; err != nil {
			return err
		}
		updatedTask = &task
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updatedTask, nil
}

func (g *GORMTaskDAO) List(ctx context.Context, bizID int64, offset, limit int) ([]*Task, error) {
	var tasks []*Task
	err := g.db.WithContext(ctx).
		Where("biz_id = ?", bizID).
		Order("id DESC").
		Offset(offset).
		Limit(limit).
		Find(&tasks).Error
	if err != nil {
		return nil, err
	}
	return tasks, nil
}

func (g *GORMTaskDAO) Count(ctx context.Context, bizID int64) (int64, error) {
	var count int64
	err := g.db.WithContext(ctx).Model(&Task{}).Where("biz_id = ?", bizID).Count(&count).Error
	return count, err
}

func (g *GORMTaskDAO) Update(ctx context.Context, task Task) error {
	return g.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&Task{}).Where("id = ?", task.ID).Updates(taskUpdateFields(task))
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return fmt.Errorf("更新失败：该任务不存在 (ID=%d)", task.ID)
		}
		if err := replaceOverrideRules(tx, task.TenantID, task.ID, task.ParamOverrideRules); err != nil {
			return err
		}
		return replaceExecutionNotificationRules(tx, task.TenantID, task.ID, task.NotificationRules)
	})
}

func taskUpdateFields(task Task) map[string]any {
	return map[string]any{
		"name":                  task.Name,
		"runner_id":             task.RunnerID,
		"type":                  task.Type,
		"cron_expr":             task.CronExpr,
		"grpc_config":           task.GrpcConfig,
		"program":               task.Program,
		"http_config":           task.HTTPConfig,
		"retry_config":          task.RetryConfig,
		"schedule_params":       task.ScheduleParams,
		"max_execution_seconds": task.MaxExecutionSeconds,
		"next_time":             task.NextTime,
		"version":               gorm.Expr("version + 1"),
		"utime":                 time.Now().UnixMilli(),
		"metadata":              task.Metadata,
	}
}

func (g *GORMTaskDAO) Delete(ctx context.Context, id int64) error {
	return g.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("task_id = ?", id).Delete(&TaskParamOverrideRule{}).Error; err != nil {
			return err
		}
		if err := tx.Where("task_id = ?", id).Delete(&TaskRunParamOverride{}).Error; err != nil {
			return err
		}
		if err := tx.Where("task_id = ?", id).Delete(&TaskExecutionNotificationRule{}).Error; err != nil {
			return err
		}
		return tx.Delete(&Task{}, id).Error
	})
}
