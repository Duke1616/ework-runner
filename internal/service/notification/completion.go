package notification

import (
	"context"
	"fmt"

	notificationv1 "github.com/Duke1616/etask/api/proto/gen/ealert/notification/v1"
	"github.com/Duke1616/etask/internal/domain"
)

// CompletionNotifier 负责投递任务执行终态通知。
type CompletionNotifier interface {
	// Notify 根据已命中的通知规则投递执行终态快照。
	Notify(ctx context.Context, rule domain.ExecutionNotificationRule, execution domain.TaskExecution) error
}

type alertCompletionNotifier struct {
	notificationClient notificationv1.NotificationServiceClient
}

// NewEAlertCompletionNotifier 创建基于 EAlert 的任务执行通知器。
func NewEAlertCompletionNotifier(notificationClient notificationv1.NotificationServiceClient) CompletionNotifier {
	return &alertCompletionNotifier{notificationClient: notificationClient}
}

// Notify 构造任务执行通知请求，调用 EAlert 并校验每个渠道的投递结果。
func (n *alertCompletionNotifier) Notify(ctx context.Context, rule domain.ExecutionNotificationRule,
	execution domain.TaskExecution) error {
	if n == nil || n.notificationClient == nil {
		return fmt.Errorf("EAlert 通知服务客户端不能为空")
	}
	request, err := buildDispatchNotificationRequest(rule, execution)
	if err != nil {
		return err
	}

	response, err := n.notificationClient.DispatchNotification(ctx, request)
	if err != nil {
		return fmt.Errorf("调用 EAlert 投递任务通知失败: %w", err)
	}

	return validateDispatchNotificationResponse(response)
}
