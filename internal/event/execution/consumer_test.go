package execution_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/event/execution"
	executionmocks "github.com/Duke1616/etask/internal/event/execution/mocks"
	"github.com/ecodeclub/mq-api"
	"go.uber.org/mock/gomock"
)

func TestEventConsumerConsume(t *testing.T) {
	successState := domain.ExecutionState{ID: 10, TaskID: 20, Status: domain.TaskExecutionStatusSuccess}
	testCases := []struct {
		name    string
		message *mq.Message
		prepare func(*executionmocks.MockEventHandler)
		wantErr string
	}{
		{
			name: "增量日志只写入日志服务",
			message: eventMessage(t, execution.NewLogBatchEvent(execution.LogBatch{
				DispatchID: "dispatch-1", Sequence: 1,
				State: domain.ExecutionState{ID: 10, TaskID: 20}, Logs: []string{"第一行", "第二行"},
			})),
			prepare: func(handler *executionmocks.MockEventHandler) {
				handler.EXPECT().AppendExecutionLogs(gomock.Any(), int64(10), int64(20), []string{"第一行", "第二行"}).Return(nil)
			},
		},
		{
			name: "终态补写尾部日志后更新状态",
			message: eventMessage(t, execution.NewFinishedEvent(execution.Finished{
				DispatchID: "dispatch-1", State: successState, PendingLogs: []string{"发送失败"},
			})),
			prepare: func(handler *executionmocks.MockEventHandler) {
				handler.EXPECT().AppendExecutionLogs(gomock.Any(), int64(10), int64(20), []string{"发送失败"}).Return(nil)
				handler.EXPECT().FindByID(gomock.Any(), int64(10)).Return(domain.TaskExecution{
					ID: 10, Status: domain.TaskExecutionStatusRunning,
				}, nil)
				handler.EXPECT().UpdateState(gomock.Any(), successState).Return(nil)
			},
		},
		{
			name: "已终态执行忽略重复状态",
			message: eventMessage(t, execution.NewFinishedEvent(execution.Finished{
				DispatchID: "dispatch-1", State: successState,
			})),
			prepare: func(handler *executionmocks.MockEventHandler) {
				handler.EXPECT().FindByID(gomock.Any(), int64(10)).Return(domain.TaskExecution{
					ID: 10, Status: domain.TaskExecutionStatusSuccess,
				}, nil)
			},
		},
		{name: "非法 JSON", message: &mq.Message{Value: []byte("{")}, wantErr: "解析 Agent 执行事件失败"},
		{name: "拒绝无版本事件", message: eventMessage(t, execution.Event{}), wantErr: "版本非法"},
		{
			name: "执行记录查询失败",
			message: eventMessage(t, execution.NewFinishedEvent(execution.Finished{
				DispatchID: "dispatch-1", State: successState,
			})),
			prepare: func(handler *executionmocks.MockEventHandler) {
				handler.EXPECT().FindByID(gomock.Any(), int64(10)).Return(domain.TaskExecution{}, errors.New("查询失败"))
			},
			wantErr: "查询失败",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			handler := executionmocks.NewMockEventHandler(ctrl)
			if testCase.prepare != nil {
				testCase.prepare(handler)
			}
			err := execution.NewEventConsumer(handler).Consume(context.Background(), testCase.message)
			if testCase.wantErr == "" && err != nil {
				t.Fatalf("Consume() 返回意外错误: %v", err)
			}
			if testCase.wantErr != "" && (err == nil || !strings.Contains(err.Error(), testCase.wantErr)) {
				t.Fatalf("Consume() 错误 = %v, 期望包含 %q", err, testCase.wantErr)
			}
		})
	}
}

func TestEventConsumerDeduplicatesLogEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	handler := executionmocks.NewMockEventHandler(ctrl)
	handler.EXPECT().AppendExecutionLogs(gomock.Any(), int64(10), int64(0), []string{"日志"}).Return(nil)
	consumer := execution.NewEventConsumer(handler)
	message := eventMessage(t, execution.NewLogBatchEvent(execution.LogBatch{
		DispatchID: "dispatch-1", Sequence: 1,
		State: domain.ExecutionState{ID: 10}, Logs: []string{"日志"},
	}))
	if err := consumer.Consume(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	if err := consumer.Consume(context.Background(), message); err != nil {
		t.Fatal(err)
	}
}

func eventMessage(t *testing.T, event execution.Event) *mq.Message {
	t.Helper()
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	return &mq.Message{Value: data}
}
