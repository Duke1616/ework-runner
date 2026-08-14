package notification

import (
	_ "embed"

	notificationv1 "github.com/Duke1616/etask/api/proto/gen/ealert/notification/v1"
)

const (
	// BuiltinTaskExecutionTemplateSetKey 是任务执行通知内置模板集在 EAlert 中的稳定业务 key。
	BuiltinTaskExecutionTemplateSetKey = "etask.task.execution.completed"
	defaultTemplateName                = "任务执行通知"
	defaultTemplateDesc                = "ETask 任务执行终态通知"
	defaultTemplateVersion             = "v1.0.0"
)

//go:embed fs/task_execution_lark_card.tmpl
var defaultLarkCardTemplate string

type templateConfig struct {
	name        string
	description string
	channel     notificationv1.Channel
	versionName string
	content     string
	key         string
	business    notificationv1.Business
}

var builtinTemplates = []templateConfig{
	{
		name:        defaultTemplateName,
		description: defaultTemplateDesc,
		channel:     notificationv1.Channel_LARK_CARD,
		versionName: defaultTemplateVersion,
		content:     defaultLarkCardTemplate,
		key:         BuiltinTaskExecutionTemplateSetKey,
		business:    notificationv1.Business_TASK,
	},
}
