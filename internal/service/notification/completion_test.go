package notification

import (
	"context"
	"testing"

	notificationv1 "github.com/Duke1616/etask/api/proto/gen/ealert/notification/v1"
	templatev1 "github.com/Duke1616/etask/api/proto/gen/ealert/template/v1"
	"github.com/Duke1616/etask/internal/domain"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type notificationClientStub struct {
	request *notificationv1.DispatchNotificationRequest
	calls   int
}

type templateClientStub struct {
	templatev1.TemplateServiceClient
	response *templatev1.ResolveTemplateIDResponse
}

func (s *templateClientStub) ResolveTemplateID(_ context.Context, _ *templatev1.ResolveTemplateIDRequest,
	_ ...grpc.CallOption) (*templatev1.ResolveTemplateIDResponse, error) {
	return s.response, nil
}

func (s *notificationClientStub) DispatchNotification(_ context.Context,
	req *notificationv1.DispatchNotificationRequest,
	_ ...grpc.CallOption) (*notificationv1.DispatchNotificationResponse, error) {
	s.calls++
	s.request = req
	return &notificationv1.DispatchNotificationResponse{
		Results: []*notificationv1.NotificationDispatchResult{{
			Channel: notificationv1.Channel_EMAIL, Status: notificationv1.SendStatus_SUCCEEDED,
		}},
	}, nil
}

func TestEAlertCompletionNotifierBuildsExecutionSnapshot(t *testing.T) {
	client := &notificationClientStub{}
	notifier := NewEAlertCompletionNotifier(client, nil)
	rule := domain.ExecutionNotificationRule{
		TriggerStatus: domain.TaskExecutionStatusSuccess, TemplateSetID: 42, Enabled: true,
		Recipients: []domain.NotificationRecipient{{
			Type: domain.NotificationRecipientUser, TargetIDs: []int64{101, 102},
		}},
		Channels: []domain.NotificationChannel{domain.NotificationChannelEmail},
	}
	execution := domain.TaskExecution{
		ID: 40, TenantID: 20, Source: domain.TaskExecutionSourceTask,
		Status: domain.TaskExecutionStatusSuccess, TaskResult: `{"changed":2}`,
		StartTime: 1000, EndTime: 2500, RetryCount: 1, ExecutorNodeID: "executor-1",
		Task: domain.Task{
			ID: 10, Name: "snapshot-name", Type: domain.TaskTypeRecurring,
			CronExpr: "0 * * * * *", ScheduleParams: map[string]string{"region": "cn"},
		},
	}

	err := notifier.Notify(t.Context(), rule, execution)

	require.NoError(t, err)
	require.Equal(t, 1, client.calls)
	require.Equal(t, notificationv1.Business_TASK, client.request.GetBusiness())
	require.Equal(t, "etask:execution:40:completed", client.request.GetIdempotencyKey())
	require.Equal(t, int64(42), client.request.GetTemplateSetId())
	require.Empty(t, client.request.GetTemplateSetKey())
	require.Equal(t, []int64{101, 102}, client.request.GetRecipients()[0].GetTargetIds())
	params := client.request.GetTemplateParams().AsMap()
	require.Equal(t, "[任务通知] 任务 snapshot-name 执行成功", params["Subject"])
	require.Equal(t, "snapshot-name", params["Task"].(map[string]any)["Name"])
	require.Equal(t, "周期任务", params["Task"].(map[string]any)["TypeText"])
	require.Equal(t, "40", params["Execution"].(map[string]any)["ID"])
	require.Equal(t, "任务调度", params["Execution"].(map[string]any)["SourceText"])
	require.Equal(t, float64(1500), params["Execution"].(map[string]any)["DurationMillis"])
	require.Equal(t, "cn", params["ScheduleParams"].(map[string]any)["region"])
}

func TestEAlertCompletionNotifierRejectsMissingTaskSnapshot(t *testing.T) {
	client := &notificationClientStub{}
	notifier := NewEAlertCompletionNotifier(client, nil)
	rule := domain.ExecutionNotificationRule{TemplateSetID: 42, Enabled: true}

	err := notifier.Notify(t.Context(), rule, domain.TaskExecution{ID: 40})

	require.ErrorContains(t, err, "执行快照缺少任务 ID")
	require.Zero(t, client.calls)
}

func TestEAlertCompletionNotifierResolvesBuiltinTemplate(t *testing.T) {
	client := &notificationClientStub{}
	templateClient := &templateClientStub{response: &templatev1.ResolveTemplateIDResponse{TemplateSetId: 99}}
	notifier := NewEAlertCompletionNotifier(client, templateClient)
	rule := domain.ExecutionNotificationRule{
		TriggerStatus: domain.TaskExecutionStatusSuccess,
		TemplateSetID: 0,
		Recipients: []domain.NotificationRecipient{{
			Type: domain.NotificationRecipientUser, TargetIDs: []int64{101},
		}},
		Channels: []domain.NotificationChannel{domain.NotificationChannelLarkCard},
		Enabled:  true,
	}
	execution := domain.TaskExecution{
		ID:     40,
		Status: domain.TaskExecutionStatusSuccess,
		Task:   domain.Task{ID: 10, Name: "builtin-template"},
	}

	require.NoError(t, notifier.Notify(t.Context(), rule, execution))
	require.Equal(t, int64(99), client.request.GetTemplateSetId())
	require.Empty(t, client.request.GetTemplateSetKey())
}

func TestValidateDispatchNotificationResponse(t *testing.T) {
	tests := []struct {
		name     string
		response *notificationv1.DispatchNotificationResponse
		wantErr  string
	}{
		{
			name:    "nil response",
			wantErr: "EAlert 返回空响应",
		},
		{
			name: "request error",
			response: &notificationv1.DispatchNotificationResponse{
				ErrorCode:    notificationv1.ErrorCode_INVALID_PARAMETER,
				ErrorMessage: "invalid template",
			},
			wantErr: "EAlert 处理任务通知失败",
		},
		{
			name: "empty channel result",
			response: &notificationv1.DispatchNotificationResponse{
				Results: []*notificationv1.NotificationDispatchResult{nil},
			},
			wantErr: "EAlert 返回空渠道结果",
		},
		{
			name: "channel error",
			response: &notificationv1.DispatchNotificationResponse{
				Results: []*notificationv1.NotificationDispatchResult{{
					Channel:      notificationv1.Channel_EMAIL,
					ErrorCode:    notificationv1.ErrorCode_SEND_NOTIFICATION_FAILED,
					ErrorMessage: "provider failed",
				}},
			},
			wantErr: "EAlert 渠道投递失败",
		},
		{
			name: "channel failed status",
			response: &notificationv1.DispatchNotificationResponse{
				Results: []*notificationv1.NotificationDispatchResult{{
					Channel: notificationv1.Channel_EMAIL,
					Status:  notificationv1.SendStatus_FAILED,
				}},
			},
			wantErr: "EAlert 渠道投递失败",
		},
		{
			name: "success",
			response: &notificationv1.DispatchNotificationResponse{
				Results: []*notificationv1.NotificationDispatchResult{{
					Channel: notificationv1.Channel_EMAIL,
					Status:  notificationv1.SendStatus_SUCCEEDED,
				}},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateDispatchNotificationResponse(test.response)
			if test.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, test.wantErr)
		})
	}
}
