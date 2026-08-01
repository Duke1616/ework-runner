package complete

import (
	"context"
	"testing"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/event"
	"github.com/Duke1616/etask/internal/service/task"
	"github.com/stretchr/testify/require"
)

type completionExecutionServiceStub struct {
	task.ExecutionService
	updated      bool
	targetStatus domain.TaskExecutionStatus
}

func (s *completionExecutionServiceStub) UpdateScheduleResult(_ context.Context, _ int64,
	_ []domain.TaskExecutionStatus, status domain.TaskExecutionStatus, _ int32, _ int64,
	_ map[string]string, _ string, _ string) (bool, error) {
	s.targetStatus = status
	return s.updated, nil
}

func TestConsumerIgnoresCompletionWithoutStateTransition(t *testing.T) {
	consumer := &Consumer{execSvc: &completionExecutionServiceStub{updated: false}}

	err := consumer.handleTask(t.Context(), event.Event{
		ExecID:     10,
		TaskID:     20,
		ExecStatus: domain.TaskExecutionStatusSuccess,
	})

	require.NoError(t, err)
}

func TestConsumerPersistsCancelledCompletion(t *testing.T) {
	executions := &completionExecutionServiceStub{updated: false}
	consumer := &Consumer{execSvc: executions}

	err := consumer.handleTask(t.Context(), event.Event{
		ExecID: 10, ExecStatus: domain.TaskExecutionStatusCancelled,
	})

	require.NoError(t, err)
	require.Equal(t, domain.TaskExecutionStatusCancelled, executions.targetStatus)
}
