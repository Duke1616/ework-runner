package notification

import (
	"fmt"

	notificationv1 "github.com/Duke1616/etask/api/proto/gen/ealert/notification/v1"
	"github.com/Duke1616/etask/internal/domain"
	"github.com/samber/lo"
)

// validateDispatchNotificationResponse 校验 EAlert 请求级和渠道级的处理结果。
func validateDispatchNotificationResponse(response *notificationv1.DispatchNotificationResponse) error {
	if response == nil {
		return fmt.Errorf("EAlert 返回空响应")
	}
	if response.GetErrorCode() != notificationv1.ErrorCode_ERROR_CODE_UNSPECIFIED {
		return fmt.Errorf("EAlert 处理任务通知失败: code=%s message=%s",
			response.GetErrorCode(), response.GetErrorMessage())
	}

	for _, result := range response.GetResults() {
		if result == nil {
			return fmt.Errorf("EAlert 返回空渠道结果")
		}
		if result.GetErrorCode() != notificationv1.ErrorCode_ERROR_CODE_UNSPECIFIED ||
			result.GetStatus() == notificationv1.SendStatus_FAILED {
			return fmt.Errorf("EAlert 渠道投递失败: channel=%s code=%s message=%s",
				result.GetChannel(), result.GetErrorCode(), result.GetErrorMessage())
		}
	}
	return nil
}

// toProtoRecipients 将领域接收对象转换为 EAlert 接收对象选择器。
func toProtoRecipients(recipients []domain.NotificationRecipient) []*notificationv1.RecipientSelector {
	return lo.Map(recipients, toProtoRecipient)
}

// toProtoChannels 将领域投递渠道转换为 EAlert 渠道枚举。
func toProtoChannels(channels []domain.NotificationChannel) []notificationv1.Channel {
	return lo.Map(channels, toProtoChannel)
}

func toProtoRecipient(recipient domain.NotificationRecipient, _ int) *notificationv1.RecipientSelector {
	return &notificationv1.RecipientSelector{
		Type: notificationv1.RecipientSelectorType(
			notificationv1.RecipientSelectorType_value[string(recipient.Type)]),
		TargetIds: recipient.TargetIDs,
	}
}

func toProtoChannel(channel domain.NotificationChannel, _ int) notificationv1.Channel {
	return notificationv1.Channel(notificationv1.Channel_value[string(channel)])
}
