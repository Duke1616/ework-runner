package notification

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	notificationv1 "github.com/Duke1616/etask/api/proto/gen/ealert/notification/v1"
	"github.com/Duke1616/etask/internal/domain"
	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/structpb"
)

// buildDispatchNotificationRequest 将领域通知规则和执行快照组装为 EAlert 请求。
func buildDispatchNotificationRequest(rule domain.ExecutionNotificationRule,
	execution domain.TaskExecution, templateSetID int64) (*notificationv1.DispatchNotificationRequest, error) {
	if err := validateExecutionSnapshot(execution); err != nil {
		return nil, err
	}
	if err := rule.Validate(); err != nil {
		return nil, fmt.Errorf("任务执行通知规则无效: %w", err)
	}
	if templateSetID <= 0 {
		return nil, fmt.Errorf("任务执行通知缺少有效的模板集 ID")
	}

	templateParams, err := buildCompletionTemplateParams(execution)
	if err != nil {
		return nil, err
	}

	return &notificationv1.DispatchNotificationRequest{
		Business:       notificationv1.Business_TASK,
		IdempotencyKey: fmt.Sprintf("etask:execution:%d:completed", execution.ID),
		Recipients:     toProtoRecipients(rule.Recipients),
		Channels:       toProtoChannels(rule.Channels),
		TemplateSetId:  templateSetID,
		TemplateParams: templateParams,
	}, nil
}

// validateExecutionSnapshot 检查构造通知所需的最小执行快照。
func validateExecutionSnapshot(execution domain.TaskExecution) error {
	if execution.ID <= 0 {
		return fmt.Errorf("执行快照缺少有效的执行 ID: %d", execution.ID)
	}
	if execution.Task.ID <= 0 {
		return fmt.Errorf("执行快照缺少任务 ID: execution_id=%d", execution.ID)
	}
	return nil
}

// buildCompletionTemplateParams 构造传递给 EAlert 模板的稳定参数。
func buildCompletionTemplateParams(execution domain.TaskExecution) (*structpb.Struct, error) {
	title := executionNotificationTitle(execution.Task.Name, execution.Status)

	params, err := structpb.NewStruct(map[string]any{
		"Title":          title,
		"Subject":        "[任务通知] " + title,
		"MessageTitle":   "📢 [任务通知] " + title,
		"Task":           buildTaskTemplateParams(execution.Task),
		"Execution":      buildExecutionTemplateParams(execution),
		"ScheduleParams": stringMapToAny(execution.Task.ScheduleParams),
	})
	if err != nil {
		return nil, fmt.Errorf("构造 EAlert 模板参数失败: %w", err)
	}
	return params, nil
}

// buildTaskTemplateParams 构造不包含敏感配置的任务模板快照。
func buildTaskTemplateParams(task domain.Task) map[string]any {
	return map[string]any{
		"ID":       strconv.FormatInt(task.ID, 10),
		"Name":     task.Name,
		"Type":     task.Type.String(),
		"TypeText": taskTypeText(task.Type),
		"CronExpr": task.CronExpr,
	}
}

// buildExecutionTemplateParams 构造执行状态、时间和结果模板快照。
func buildExecutionTemplateParams(execution domain.TaskExecution) map[string]any {
	durationMillis := executionDurationMillis(execution)
	return map[string]any{
		"ID":             strconv.FormatInt(execution.ID, 10),
		"Status":         execution.Status.String(),
		"StatusText":     executionStatusText(execution.Status),
		"Result":         execution.TaskResult,
		"ResultData":     parseResultData(execution.TaskResult),
		"Source":         execution.Source.String(),
		"SourceText":     executionSourceText(execution.Source),
		"RequestID":      execution.RequestID,
		"StartTime":      execution.StartTime,
		"StartedAt":      formatUnixMillis(execution.StartTime),
		"EndTime":        execution.EndTime,
		"EndedAt":        formatUnixMillis(execution.EndTime),
		"DurationMillis": durationMillis,
		"Duration":       (time.Duration(durationMillis) * time.Millisecond).String(),
		"RetryCount":     execution.RetryCount,
		"ExecutorNodeID": execution.ExecutorNodeID,
	}
}

// executionDurationMillis 返回执行耗时，并将异常的负数时间差归零。
func executionDurationMillis(execution domain.TaskExecution) int64 {
	if execution.EndTime <= execution.StartTime {
		return 0
	}
	return execution.EndTime - execution.StartTime
}

// executionNotificationTitle 生成邮件、飞书等渠道共用的通知标题。
func executionNotificationTitle(taskName string, status domain.TaskExecutionStatus) string {
	return fmt.Sprintf("任务 %s 执行%s", taskName, executionStatusText(status))
}

// executionStatusText 返回面向用户的执行状态文案。
func executionStatusText(status domain.TaskExecutionStatus) string {
	switch status {
	case domain.TaskExecutionStatusSuccess:
		return "成功"
	case domain.TaskExecutionStatusFailed:
		return "失败"
	case domain.TaskExecutionStatusCancelled:
		return "已取消"
	default:
		return status.String()
	}
}

// taskTypeText 返回面向用户的任务类型文案。
func taskTypeText(taskType domain.TaskType) string {
	switch taskType {
	case domain.TaskTypeRecurring:
		return "周期任务"
	case domain.TaskTypeOneTime:
		return "一次性任务"
	default:
		return taskType.String()
	}
}

// executionSourceText 返回面向用户的执行来源文案。
func executionSourceText(source domain.TaskExecutionSource) string {
	switch source {
	case domain.TaskExecutionSourceTask:
		return "任务调度"
	case domain.TaskExecutionSourceCodebookPreview:
		return "Codebook 试运行"
	case domain.TaskExecutionSourceWorkflow:
		return "工作流"
	default:
		return source.String()
	}
}

// formatUnixMillis 将毫秒时间戳格式化为本地通知时间。
func formatUnixMillis(value int64) string {
	if value <= 0 {
		return ""
	}
	return time.UnixMilli(value).In(time.Local).Format("2006-01-02 15:04:05")
}

// parseResultData 仅解析合法 JSON，供飞书等结构化模板使用。
func parseResultData(result string) any {
	if result == "" {
		return nil
	}
	var value any
	if err := json.Unmarshal([]byte(result), &value); err != nil {
		return nil
	}
	return value
}

// stringMapToAny 将调度参数转换为 Struct 可接受的值类型。
func stringMapToAny(src map[string]string) map[string]any {
	return lo.MapValues(src, func(value string, _ string) any {
		return value
	})
}
