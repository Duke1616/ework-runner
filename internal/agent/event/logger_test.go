package event

import (
	"context"
	"errors"
	"testing"
	"time"

	eventmocks "github.com/Duke1616/etask/internal/agent/event/mocks"
	"github.com/Duke1616/etask/internal/domain"
	executionevent "github.com/Duke1616/etask/internal/event/execution"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestKafkaTaskLoggerFlushesByCapacity(t *testing.T) {
	ctrl := gomock.NewController(t)
	publisher := eventmocks.NewMockExecutionEventPublisher(ctrl)
	produced := make(chan struct{}, 1)
	publisher.EXPECT().PublishLogs(gomock.Any(), executionevent.LogBatch{
		DispatchID: "dispatch-1", Sequence: 1,
		State: domain.ExecutionState{ID: 10, TaskID: 20, TaskName: "测试任务"},
		Logs:  []string{"第一行", "第二行"},
	}).DoAndReturn(func(context.Context, executionevent.LogBatch) error {
		produced <- struct{}{}
		return nil
	})
	logger := newKafkaTaskLoggerWithOptions(context.Background(), publisher, logCommand(), nil, 2, time.Hour)
	logger.Log("第一行")
	logger.Log("第二行")
	select {
	case <-produced:
	case <-time.After(time.Second):
		t.Fatal("日志达到容量后没有及时发布")
	}
	logger.Close()

	require.Empty(t, logger.PendingLogs())
}

func TestKafkaTaskLoggerKeepsFailedBatchForFinalResult(t *testing.T) {
	ctrl := gomock.NewController(t)
	publisher := eventmocks.NewMockExecutionEventPublisher(ctrl)
	publisher.EXPECT().PublishLogs(gomock.Any(), executionevent.LogBatch{
		DispatchID: "dispatch-1", Sequence: 1,
		State: domain.ExecutionState{ID: 10, TaskID: 20, TaskName: "测试任务"},
		Logs:  []string{"需要兜底"},
	}).Return(errors.New("Kafka 不可用"))
	logger := newKafkaTaskLoggerWithOptions(context.Background(), publisher, logCommand(), nil, 10, time.Hour)
	logger.Log("需要兜底")
	logger.Close()
	if logs := logger.PendingLogs(); len(logs) != 1 || logs[0] != "需要兜底" {
		t.Fatalf("未发送日志 = %#v", logs)
	}
}

func logCommand() ExecuteCommand {
	return ExecuteCommand{
		DispatchID: "dispatch-1", ExecutionID: 10, TaskID: 20, TaskName: "测试任务",
		TenantID: 30, Source: domain.TaskExecutionSourceTask, Handler: "shell",
	}
}
