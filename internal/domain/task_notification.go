package domain

import (
	"fmt"
	"strings"

	"github.com/Duke1616/etask/internal/errs"
)

// NotificationRecipientType 表示 EAlert 接收对象解析规则。
type NotificationRecipientType string

const (
	NotificationRecipientUser              NotificationRecipientType = "RECIPIENT_USER"
	NotificationRecipientTeam              NotificationRecipientType = "RECIPIENT_TEAM"
	NotificationRecipientDepartment        NotificationRecipientType = "RECIPIENT_DEPARTMENT"
	NotificationRecipientOnCall            NotificationRecipientType = "RECIPIENT_ONCALL"
	NotificationRecipientDepartmentLeader  NotificationRecipientType = "RECIPIENT_DEPARTMENT_LEADER"
	NotificationRecipientSupervisingLeader NotificationRecipientType = "RECIPIENT_SUPERVISING_LEADER"
)

// IsValid 判断接收对象解析规则是否受 EAlert 支持。
func (t NotificationRecipientType) IsValid() bool {
	switch t {
	case NotificationRecipientUser,
		NotificationRecipientTeam,
		NotificationRecipientDepartment,
		NotificationRecipientOnCall,
		NotificationRecipientDepartmentLeader,
		NotificationRecipientSupervisingLeader:
		return true
	default:
		return false
	}
}

// NotificationChannel 表示 EAlert 投递渠道。
type NotificationChannel string

const (
	NotificationChannelLarkCard NotificationChannel = "LARK_CARD"
	NotificationChannelEmail    NotificationChannel = "EMAIL"
	NotificationChannelInApp    NotificationChannel = "IN_APP"
	NotificationChannelWechat   NotificationChannel = "WECHAT"
)

// IsValid 判断投递渠道是否受 EAlert 支持。
func (c NotificationChannel) IsValid() bool {
	switch c {
	case NotificationChannelLarkCard,
		NotificationChannelEmail,
		NotificationChannelInApp,
		NotificationChannelWechat:
		return true
	default:
		return false
	}
}

// NotificationRecipient 保存一种接收对象及其已标准化 ID。
type NotificationRecipient struct {
	Type      NotificationRecipientType `json:"type"`       // 接收对象类型，决定 ID 所属系统和解析方式。
	TargetIDs []int64                   `json:"target_ids"` // 调用方预先转换的数值 ID。
}

// ExecutionNotificationRule 定义一个执行终态对应的通知行为。
// 规则由任务 ID 和 TriggerStatus 唯一标识，不对外暴露数据库行 ID。
type ExecutionNotificationRule struct {
	TriggerStatus  TaskExecutionStatus     `json:"trigger_status"`   // 触发通知的执行终态。
	Recipients     []NotificationRecipient `json:"recipients"`       // 接收对象规则。
	Channels       []NotificationChannel   `json:"channels"`         // 投递渠道。
	TemplateSetKey string                  `json:"template_set_key"` // 模板集稳定业务 key，内置和自定义模板均通过该 key 标识。
	Enabled        bool                    `json:"enabled"`          // 是否启用规则。
}

// EnabledNotificationRule 返回指定终态命中的启用规则。
func (t *Task) EnabledNotificationRule(status TaskExecutionStatus) (ExecutionNotificationRule, bool) {
	for _, rule := range t.NotificationRules {
		if rule.Enabled && rule.TriggerStatus == status {
			return rule, true
		}
	}
	return ExecutionNotificationRule{}, false
}

// ValidateNotificationRules 校验任务执行通知规则是否满足领域约束。
func (t *Task) ValidateNotificationRules() error {
	statuses := make(map[TaskExecutionStatus]struct{}, len(t.NotificationRules))
	for i, rule := range t.NotificationRules {
		if err := rule.Validate(); err != nil {
			return fmt.Errorf("execution_notifications[%d]: %w", i, err)
		}
		if _, exists := statuses[rule.TriggerStatus]; exists {
			return fmt.Errorf("%w: execution_notifications 包含重复终态 %s",
				errs.ErrInvalidParameter, rule.TriggerStatus)
		}
		statuses[rule.TriggerStatus] = struct{}{}
	}
	return nil
}

// Validate 校验单条执行通知规则。
func (r ExecutionNotificationRule) Validate() error {
	if !r.TriggerStatus.IsTerminalStatus() {
		return fmt.Errorf("%w: trigger_status 仅支持 FAILED、SUCCESS 和 CANCELLED: %s",
			errs.ErrInvalidParameter, r.TriggerStatus)
	}
	templateSetKey := strings.TrimSpace(r.TemplateSetKey)
	if templateSetKey == "" {
		return fmt.Errorf("%w: template_set_key 不能为空", errs.ErrInvalidParameter)
	}
	if len(templateSetKey) > 128 {
		return fmt.Errorf("%w: template_set_key 长度不能超过 128", errs.ErrInvalidParameter)
	}
	if len(r.Recipients) == 0 {
		return fmt.Errorf("%w: recipients 不能为空", errs.ErrInvalidParameter)
	}
	for i, recipient := range r.Recipients {
		if err := recipient.Validate(); err != nil {
			return fmt.Errorf("recipients[%d]: %w", i, err)
		}
	}

	if len(r.Channels) == 0 {
		return fmt.Errorf("%w: channels 不能为空", errs.ErrInvalidParameter)
	}
	channels := make(map[NotificationChannel]struct{}, len(r.Channels))
	for _, channel := range r.Channels {
		if !channel.IsValid() {
			return fmt.Errorf("%w: channels 包含不受支持的渠道 %s",
				errs.ErrInvalidParameter, channel)
		}
		if _, exists := channels[channel]; exists {
			return fmt.Errorf("%w: channels 包含重复渠道 %s",
				errs.ErrInvalidParameter, channel)
		}
		channels[channel] = struct{}{}
	}
	return nil
}

// Validate 校验接收对象类型及目标 ID。
func (r NotificationRecipient) Validate() error {
	if !r.Type.IsValid() {
		return fmt.Errorf("%w: type 不受支持: %s", errs.ErrInvalidParameter, r.Type)
	}
	if len(r.TargetIDs) == 0 {
		return fmt.Errorf("%w: target_ids 不能为空", errs.ErrInvalidParameter)
	}
	for _, id := range r.TargetIDs {
		if id <= 0 {
			return fmt.Errorf("%w: target_ids 必须为正整数", errs.ErrInvalidParameter)
		}
	}
	return nil
}
