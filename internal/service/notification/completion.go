package notification

import (
	"context"
	"fmt"

	notificationv1 "github.com/Duke1616/etask/api/proto/gen/ealert/notification/v1"
	templatev1 "github.com/Duke1616/etask/api/proto/gen/ealert/template/v1"
	"github.com/Duke1616/etask/internal/domain"
)

// CompletionNotifier 负责投递任务执行终态通知。
type CompletionNotifier interface {
	// Notify 根据已命中的通知规则投递执行终态快照。
	Notify(ctx context.Context, rule domain.ExecutionNotificationRule, execution domain.TaskExecution) error
}

type alertCompletionNotifier struct {
	notificationClient notificationv1.NotificationServiceClient
	templateClient     templatev1.TemplateServiceClient
}

// NewEAlertCompletionNotifier 创建基于 EAlert 的任务执行通知器。
func NewEAlertCompletionNotifier(notificationClient notificationv1.NotificationServiceClient,
	templateClient templatev1.TemplateServiceClient) CompletionNotifier {
	return &alertCompletionNotifier{notificationClient: notificationClient, templateClient: templateClient}
}

// Notify 构造任务执行通知请求，调用 EAlert 并校验每个渠道的投递结果。
func (n *alertCompletionNotifier) Notify(ctx context.Context, rule domain.ExecutionNotificationRule,
	execution domain.TaskExecution) error {
	if n == nil || n.notificationClient == nil {
		return fmt.Errorf("EAlert 通知服务客户端不能为空")
	}
	templateSetID := rule.TemplateSetID
	if templateSetID == 0 {
		var err error
		templateSetID, err = n.resolveBuiltinTemplateSetID(ctx, rule)
		if err != nil {
			return err
		}
	}
	request, err := buildDispatchNotificationRequest(rule, execution, templateSetID)
	if err != nil {
		return err
	}

	response, err := n.notificationClient.DispatchNotification(ctx, request)
	if err != nil {
		return fmt.Errorf("调用 EAlert 投递任务通知失败: %w", err)
	}

	return validateDispatchNotificationResponse(response)
}

func (n *alertCompletionNotifier) resolveBuiltinTemplateSetID(ctx context.Context,
	rule domain.ExecutionNotificationRule) (int64, error) {
	if n.templateClient == nil {
		return 0, fmt.Errorf("ETask 默认通知模板解析客户端不能为空")
	}
	for _, channel := range rule.Channels {
		if channel != domain.NotificationChannelLarkCard {
			return 0, fmt.Errorf("ETask 默认通知模板目前仅支持飞书卡片")
		}
	}
	response, err := n.templateClient.ResolveTemplateID(ctx, &templatev1.ResolveTemplateIDRequest{
		BizId:   int64(notificationv1.Business_TASK),
		Key:     builtinTaskExecutionTemplateSetKey,
		Channel: notificationv1.Channel_LARK_CARD,
	})
	if err != nil {
		return 0, fmt.Errorf("解析 ETask 默认通知模板集失败: %w", err)
	}
	if response == nil || response.GetTemplateSetId() <= 0 {
		return 0, fmt.Errorf("解析 ETask 默认通知模板集失败: 模板集不存在")
	}
	return response.GetTemplateSetId(), nil
}
