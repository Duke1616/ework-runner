package acquirer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/repository"
	"github.com/Duke1616/etask/internal/sse"
	"github.com/stretchr/testify/require"
)

type taskRepositoryStub struct {
	repository.TaskRepository
	acquired domain.Task
	released domain.Task
	err      error
}

func (s *taskRepositoryStub) Acquire(context.Context, int64, int64, string) (domain.Task, error) {
	return s.acquired, s.err
}

func (s *taskRepositoryStub) Release(context.Context, int64, string) (domain.Task, error) {
	return s.released, s.err
}

func TestTaskAcquirerBroadcastsSuccessfulTransitions(t *testing.T) {
	hubs := sse.NewHubs(nil, "node-a")
	ch := hubs.Tasks.Subscribe(2)
	defer hubs.Tasks.Unsubscribe(2, ch)
	repo := &taskRepositoryStub{
		acquired: domain.Task{
			ID: 10, TenantID: 2, Status: domain.TaskStatusPreempted, NextTime: 100, Version: 4,
		},
		released: domain.Task{
			ID: 10, TenantID: 2, Status: domain.TaskStatusActive, NextTime: 200, Version: 5,
		},
	}
	acquirer := NewTaskAcquirer(repo, hubs)

	_, err := acquirer.Acquire(t.Context(), 10, 3, "scheduler-a")
	require.NoError(t, err)
	require.Equal(t, domain.TaskStatusPreempted.String(), receiveStatus(t, ch).Status)

	err = acquirer.Release(t.Context(), 10, "scheduler-a")
	require.NoError(t, err)
	released := receiveStatus(t, ch)
	require.Equal(t, domain.TaskStatusActive.String(), released.Status)
	require.Equal(t, int64(5), released.Version)
	require.Equal(t, int64(200), released.NextTime)
}

func TestTaskAcquirerDoesNotBroadcastFailedTransition(t *testing.T) {
	hubs := sse.NewHubs(nil, "node-a")
	ch := hubs.Tasks.Subscribe(2)
	defer hubs.Tasks.Unsubscribe(2, ch)
	acquirer := NewTaskAcquirer(&taskRepositoryStub{err: errors.New("database error")}, hubs)

	_, err := acquirer.Acquire(t.Context(), 10, 3, "scheduler-a")
	require.Error(t, err)
	select {
	case event := <-ch:
		t.Fatalf("失败的状态迁移不应广播: %#v", event)
	default:
	}
}

func receiveStatus(t *testing.T, ch <-chan sse.TaskStatusEvent) sse.TaskStatusEvent {
	t.Helper()
	select {
	case event := <-ch:
		return event
	case <-time.After(time.Second):
		t.Fatal("未收到任务状态事件")
		return sse.TaskStatusEvent{}
	}
}
