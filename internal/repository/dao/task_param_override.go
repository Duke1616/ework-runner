package dao

import (
	"context"
	"time"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/pkg/sqlx"
	"gorm.io/gorm"
)

type TaskParamOverrideInputConfig struct {
	AllowedModes []domain.TaskParamInputMode   `json:"allowed_modes"`
	DefaultMode  domain.TaskParamInputMode     `json:"default_mode"`
	SelectConfig *domain.TaskParamSelectConfig `json:"select_config,omitempty"`
}

type TaskParamOverrideRule struct {
	ID          int64                                         `gorm:"type:bigint;primaryKey;autoIncrement"`
	TenantID    int64                                         `gorm:"type:bigint unsigned;not null;uniqueIndex:uk_task_param_override,priority:1;index:idx_task_override,priority:1;comment:'租户ID'"`
	TaskID      int64                                         `gorm:"type:bigint;not null;uniqueIndex:uk_task_param_override,priority:2;index:idx_task_override,priority:2;comment:'任务ID'"`
	ParamKey    string                                        `gorm:"type:varchar(128);not null;uniqueIndex:uk_task_param_override,priority:3;comment:'参数Key'"`
	InputConfig sqlx.JSONColumn[TaskParamOverrideInputConfig] `gorm:"type:json;not null;comment:'输入模式与选项约束'"`
	Ctime       int64                                         `gorm:"comment:'创建时间'"`
	Utime       int64                                         `gorm:"comment:'更新时间'"`
}

func (TaskParamOverrideRule) TableName() string { return "task_param_override_rules" }

type TaskRunParamOverride struct {
	TaskID    int64                              `gorm:"type:bigint unsigned;primaryKey;comment:'任务ID'"`
	TenantID  int64                              `gorm:"type:bigint unsigned;not null;index;comment:'租户ID'"`
	Overrides sqlx.JSONColumn[map[string]string] `gorm:"type:json;not null;comment:'本次启动参数覆盖'"`
}

func (TaskRunParamOverride) TableName() string { return "task_run_param_overrides" }

// TaskParamOverrideDAO 负责读取任务参数覆盖相关表。
type TaskParamOverrideDAO interface {
	// FindRulesByTaskID 查询指定任务的参数覆盖规则。
	FindRulesByTaskID(ctx context.Context, taskID int64) ([]TaskParamOverrideRule, error)
	// FindPendingByTaskID 查询指定任务下一次执行待消费的参数覆盖；不存在时 found 为 false。
	FindPendingByTaskID(ctx context.Context, taskID int64) (TaskRunParamOverride, bool, error)
}

// GORMTaskParamOverrideDAO 基于 GORM 读取任务参数覆盖相关表。
type GORMTaskParamOverrideDAO struct {
	db *gorm.DB
}

// NewGORMTaskParamOverrideDAO 创建任务参数覆盖 DAO。
func NewGORMTaskParamOverrideDAO(db *gorm.DB) TaskParamOverrideDAO {
	return &GORMTaskParamOverrideDAO{db: db}
}

func replaceOverrideRules(tx *gorm.DB, tenantID, taskID int64, rules []TaskParamOverrideRule) error {
	if err := tx.Where("task_id = ?", taskID).Delete(&TaskParamOverrideRule{}).Error; err != nil {
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

// FindRulesByTaskID 查询指定任务的参数覆盖规则。
func (g *GORMTaskParamOverrideDAO) FindRulesByTaskID(ctx context.Context,
	taskID int64) ([]TaskParamOverrideRule, error) {
	var rules []TaskParamOverrideRule
	err := g.db.WithContext(ctx).
		Where("task_id = ?", taskID).
		Order("id ASC").
		Find(&rules).Error
	return rules, err
}

// FindPendingByTaskID 查询指定任务下一次执行待消费的参数覆盖。
func (g *GORMTaskParamOverrideDAO) FindPendingByTaskID(ctx context.Context,
	taskID int64) (TaskRunParamOverride, bool, error) {
	var pending TaskRunParamOverride
	err := g.db.WithContext(ctx).Where("task_id = ?", taskID).First(&pending).Error
	if err == gorm.ErrRecordNotFound {
		return TaskRunParamOverride{}, false, nil
	}
	if err != nil {
		return TaskRunParamOverride{}, false, err
	}
	return pending, true, nil
}
