package domain

import (
	"testing"

	"github.com/Duke1616/etask/internal/errs"
	"github.com/stretchr/testify/require"
)

func TestTaskValidateNotificationRules(t *testing.T) {
	valid := ExecutionNotificationRule{
		TriggerStatus:  TaskExecutionStatusSuccess,
		TemplateSetKey: "custom.task",
		Recipients: []NotificationRecipient{{
			Type: NotificationRecipientUser, TargetIDs: []int64{10},
		}},
		Channels: []NotificationChannel{NotificationChannelEmail},
		Enabled:  true,
	}
	validTask := Task{NotificationRules: []ExecutionNotificationRule{valid}}
	require.NoError(t, validTask.ValidateNotificationRules())

	duplicate := valid
	duplicateTask := Task{NotificationRules: []ExecutionNotificationRule{valid, duplicate}}
	require.ErrorContains(t, duplicateTask.ValidateNotificationRules(), "重复终态")

	invalidID := valid
	invalidID.Recipients = []NotificationRecipient{{
		Type: NotificationRecipientUser, TargetIDs: []int64{0},
	}}
	invalidTask := Task{NotificationRules: []ExecutionNotificationRule{invalidID}}
	err := invalidTask.ValidateNotificationRules()
	require.ErrorIs(t, err, errs.ErrInvalidParameter)
	require.ErrorContains(t, err, "execution_notifications[0]: recipients[0]:")
	require.ErrorContains(t, err, "target_ids 必须为正整数")
}
