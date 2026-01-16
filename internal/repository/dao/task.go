package dao

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Duke1616/ework-runner/internal/domain"
	"github.com/Duke1616/ework-runner/internal/errs"
	"github.com/Duke1616/ework-runner/pkg/sqlx"
	"gorm.io/gorm"
)

const (
	StatusActive    = "ACTIVE"
	StatusPreempted = "PREEMPTED"
	StatusInactive  = "INACTIVE"
)

// Task 任务表DAO对象
type Task struct {
	ID                  int64                               `gorm:"type:bigint;primaryKey;autoIncrement;"`
	BizID               int64                               `gorm:"type:bigint unsigned;not null;default:0;comment:biz_id"`
	Name                string                              `gorm:"type:varchar(255);not null;uniqueIndex:uniq_idx_name;comment:'任务名称'"`
	CronExpr            string                              `gorm:"type:varchar(100);not null;comment:'cron表达式'"`
	GrpcConfig          sqlx.JSONColumn[domain.GrpcConfig]  `gorm:"type:json;comment:'gRPC配置：{\"serviceName\": \"user-service\"}'"`
	HTTPConfig          sqlx.JSONColumn[domain.HTTPConfig]  `gorm:"type:json;comment:'HTTP配置：{\"endpoint\": \"https://host:port/api\"}'"`
	RetryConfig         sqlx.JSONColumn[domain.RetryConfig] `gorm:"type:json;comment:'重试配置'"`
	ScheduleParams      sqlx.JSONColumn[map[string]string]  `gorm:"type:json;comment:'每次执行要用到的基础调度参数'"`
	MaxExecutionSeconds int64                               `gorm:"type:bigint;not null;default:86400;comment:'最大执行秒数，默认24小时'"`
	ScheduleNodeID      sql.NullString                      `gorm:"type:varchar(255);index:idx_schedule_node_id_status,priority:1;comment:'当前抢占的调度节点ID'"`
	NextTime            int64                               `gorm:"type:bigint;not null;index:idx_next_time_status_utime,priority:1;comment:'下次执行时间'"`
	Status              string                              `gorm:"type:ENUM('ACTIVE', 'PREEMPTED', 'INACTIVE');not null;default:'ACTIVE';index:idx_next_time_status_utime,priority:2;index:idx_schedule_node_id_status,priority:2;comment:'任务状态: ACTIVE-可调度, PREEMPTED-已抢占, INACTIVE-停止执行。处于INACTIVE也可以被再次 ACTIVE'"`
	Version             int64                               `gorm:"type:bigint;not null;default:1;comment:'版本号，用于乐观锁'"`
	Ctime               int64                               `gorm:"comment:'创建时间'"`
	Utime               int64                               `gorm:"index:idx_next_time_status_utime,priority:3;comment:'更新时间'"`
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
	err := g.db.WithContext(ctx).Create(&task).Error
	if err != nil {
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

func (g *GORMTaskDAO) FindScheduleTasks(ctx context.Context, preemptedTimeoutMs int64, limit int) ([]*Task, error) {
	var tasks []*Task
	now := time.Now().UnixMilli()
	// 获取所有可调度的任务
	// 1. ACTIVE 状态且到了执行时间的任务
	// 2. PREEMPTED 状态但超时未续约的任务（疑似僵尸任务）
	err := g.db.WithContext(ctx).
		Where("next_time <= ? AND (status = ? OR (status = ? AND utime <= ?))",
			now, StatusActive, StatusPreempted, now-preemptedTimeoutMs).
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
