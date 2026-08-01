package task

import (
	"context"
	"errors"
	"testing"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/event"
	"github.com/Duke1616/etask/internal/repository"
	"github.com/stretchr/testify/require"
)

func TestUpdateStateIgnoresCallbackAfterTerminalState(t *testing.T) {
	repo := &terminationRepositoryStub{states: []domain.TaskExecutionStatus{
		domain.TaskExecutionStatusCancelled,
	}}
	service := &executionService{repo: repo}

	err := service.UpdateState(context.Background(), domain.ExecutionState{
		ID: 1, Status: domain.TaskExecutionStatusSuccess,
	})

	require.NoError(t, err)
}

func TestUpdateWorkflowStatePublishesCancelledCompletion(t *testing.T) {
	producer := &completeProducerStub{}
	repo := &terminationRepositoryStub{
		states: []domain.TaskExecutionStatus{domain.TaskExecutionStatusRunning},
		source: domain.TaskExecutionSourceWorkflow,
	}
	service := &executionService{repo: repo, producer: producer}

	err := service.UpdateState(context.Background(), domain.ExecutionState{
		ID: 1, Status: domain.TaskExecutionStatusCancelled, TaskResult: "管理员强制结束",
	})

	require.NoError(t, err)
	require.Equal(t, domain.TaskExecutionStatusCancelled, producer.event.ExecStatus)
	require.Equal(t, "管理员强制结束", producer.event.TaskResult)
}

type terminationRepositoryStub struct {
	repository.TaskExecutionRepository
	states []domain.TaskExecutionStatus
	reads  int
	source domain.TaskExecutionSource
}

func (s *terminationRepositoryStub) GetByID(context.Context, int64) (domain.TaskExecution, error) {
	if s.reads >= len(s.states) {
		return domain.TaskExecution{}, errors.New("unexpected GetByID call")
	}
	status := s.states[s.reads]
	s.reads++
	return domain.TaskExecution{ID: 1, Status: status, Source: s.source}, nil
}

type completeProducerStub struct {
	event event.Event
}

func (s *completeProducerStub) Produce(_ context.Context, evt event.Event) error {
	s.event = evt
	return nil
}
