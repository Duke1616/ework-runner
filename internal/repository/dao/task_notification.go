package dao

import (
	"context"
	"time"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/pkg/sqlx"
	"gorm.io/gorm"
)

// TaskExecutionNotificationRule 保存任务执行终态对应的消息通知规则。
type TaskExecutionNotificationRule struct {
	ID            int64                                           `gorm:"type:bigint;primaryKey;autoIncrement"`
	TenantID      int64                                           `gorm:"type:bigint unsigned;not null;uniqueIndex:uk_task_notification_status,priority:1;index:idx_task_notification,priority:1;comment:'租户ID'"`
	TaskID        int64                                           `gorm:"type:bigint;not null;uniqueIndex:uk_task_notification_status,priority:2;index:idx_task_notification,priority:2;comment:'任务ID'"`
	TriggerStatus string                                          `gorm:"type:varchar(32);not null;uniqueIndex:uk_task_notification_status,priority:3;comment:'触发通知的执行终态'"`
	TemplateSetID int64                                           `gorm:"type:BIGINT;not null;default:0;comment:'模板集ID，0表示使用默认模板集'"`
	Recipients    sqlx.JSONColumn[[]domain.NotificationRecipient] `gorm:"type:json;not null;comment:'接收对象规则'"`
	Channels      sqlx.JSONColumn[[]domain.NotificationChannel]   `gorm:"type:json;not null;comment:'投递渠道'"`
	Enabled       bool                                            `gorm:"not null;comment:'是否启用'"`
	Ctime         int64                                           `gorm:"comment:'创建时间'"`
	Utime         int64                                           `gorm:"comment:'更新时间'"`
}

// TableName 返回任务执行通知规则表名。
func (TaskExecutionNotificationRule) TableName() string {
	return "task_execution_notification_rules"
}

// TaskNotificationRuleDAO 负责读取任务执行通知规则表。
type TaskNotificationRuleDAO interface {
	// FindByTaskID 查询指定任务的全部执行通知规则。
	FindByTaskID(ctx context.Context, taskID int64) ([]TaskExecutionNotificationRule, error)
	// FindByTaskIDs 批量查询多个任务的执行通知规则。
	FindByTaskIDs(ctx context.Context, taskIDs []int64) ([]TaskExecutionNotificationRule, error)
}

// GORMTaskNotificationRuleDAO 基于 GORM 读取任务执行通知规则表。
type GORMTaskNotificationRuleDAO struct {
	db *gorm.DB
}

// NewGORMTaskNotificationRuleDAO 创建任务执行通知规则 DAO。
func NewGORMTaskNotificationRuleDAO(db *gorm.DB) TaskNotificationRuleDAO {
	return &GORMTaskNotificationRuleDAO{db: db}
}

func replaceExecutionNotificationRules(tx *gorm.DB, tenantID, taskID int64,
	rules []TaskExecutionNotificationRule) error {
	if err := tx.Where("task_id = ?", taskID).Delete(&TaskExecutionNotificationRule{}).Error; err != nil {
		return err
	}
	if len(rules) == 0 {
		return nil
	}
	now := time.Now().UnixMilli()
	for i := range rules {
		rules[i].ID = 0
		rules[i].TenantID = tenantID
		rules[i].TaskID = taskID
		rules[i].Ctime = now
		rules[i].Utime = now
	}
	return tx.Create(&rules).Error
}

// FindByTaskID 查询指定任务的全部执行通知规则。
func (g *GORMTaskNotificationRuleDAO) FindByTaskID(ctx context.Context,
	taskID int64) ([]TaskExecutionNotificationRule, error) {
	var rules []TaskExecutionNotificationRule
	err := g.db.WithContext(ctx).
		Where("task_id = ?", taskID).
		Order("id ASC").
		Find(&rules).Error
	return rules, err
}

// FindByTaskIDs 批量查询多个任务的执行通知规则。
func (g *GORMTaskNotificationRuleDAO) FindByTaskIDs(ctx context.Context,
	taskIDs []int64) ([]TaskExecutionNotificationRule, error) {
	if len(taskIDs) == 0 {
		return []TaskExecutionNotificationRule{}, nil
	}
	var rules []TaskExecutionNotificationRule
	err := g.db.WithContext(ctx).
		Where("task_id IN ?", taskIDs).
		Order("id ASC").
		Find(&rules).Error
	return rules, err
}
