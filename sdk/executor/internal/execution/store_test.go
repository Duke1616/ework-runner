package execution

import (
	"context"
	"testing"

	executorv1 "github.com/Duke1616/etask/api/proto/gen/etask/executor/v1"
	"github.com/stretchr/testify/require"
)

func TestStoreLifecycle(t *testing.T) {
	type state struct {
		store  *Store
		ctx    context.Context
		cancel context.CancelCauseFunc
	}
	testCases := []struct {
		name       string
		before     func(t *testing.T, state *state)
		after      func(t *testing.T, state *state)
		assertions func(t *testing.T, state *state)
	}{
		{
			name: "执行生命周期支持取消和同 ID 重试",
			assertions: func(t *testing.T, current *state) {
				initial := &executorv1.ExecutionState{Id: 1, Status: executorv1.ExecutionStatus_RUNNING}
				started, ok := current.store.Begin(initial, current.cancel)
				require.True(t, ok)
				require.Equal(t, executorv1.ExecutionStatus_RUNNING, started.GetStatus())
				_, ok = current.store.Begin(initial, func(error) {})
				require.False(t, ok)
				_, ok = current.store.Cancel(1)
				require.True(t, ok)
				require.ErrorIs(t, current.ctx.Err(), context.Canceled)
				finished, ok := current.store.Finish(1, executorv1.ExecutionStatus_FAILED_RESCHEDULABLE, "已取消")
				require.True(t, ok)
				require.Equal(t, "已取消", finished.GetTaskResult())
				_, ok = current.store.Begin(initial, func(error) {})
				require.True(t, ok)
			},
		},
		{
			name: "返回状态副本",
			assertions: func(t *testing.T, current *state) {
				started, ok := current.store.Begin(&executorv1.ExecutionState{Id: 2}, current.cancel)
				require.True(t, ok)
				started.Status = executorv1.ExecutionStatus_SUCCESS
				stored, ok := current.store.Get(2)
				require.True(t, ok)
				require.Equal(t, executorv1.ExecutionStatus_UNKNOWN, stored.GetStatus())
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			current := &state{store: NewStore()}
			current.ctx, current.cancel = context.WithCancelCause(t.Context())
			defer current.cancel(nil)
			if tc.before != nil {
				tc.before(t, current)
			}
			if tc.after != nil {
				defer tc.after(t, current)
			}
			tc.assertions(t, current)
		})
	}
}

func TestStoreTerminatePreservesCancelledTerminalState(t *testing.T) {
	store := NewStore()
	ctx, cancel := context.WithCancelCause(t.Context())
	defer cancel(nil)
	_, started := store.Begin(&executorv1.ExecutionState{
		Id: 1, Status: executorv1.ExecutionStatus_RUNNING,
	}, cancel)
	require.True(t, started)

	terminated, ok := store.Terminate(1, "管理员强制结束")
	require.True(t, ok)
	require.Equal(t, executorv1.ExecutionStatus_CANCELLED, terminated.GetStatus())
	require.ErrorIs(t, context.Cause(ctx), ErrTerminated)

	finished, ok := store.Finish(1, executorv1.ExecutionStatus_SUCCESS, "late success")
	require.True(t, ok)
	require.Equal(t, executorv1.ExecutionStatus_CANCELLED, finished.GetStatus())
	require.Equal(t, "管理员强制结束", finished.GetTaskResult())
}

func TestStoreTerminateBeforeExecuteCreatesTombstone(t *testing.T) {
	store := NewStore()

	terminated, ok := store.Terminate(9, "管理员强制结束")
	require.True(t, ok)
	require.Equal(t, executorv1.ExecutionStatus_CANCELLED, terminated.GetStatus())

	ctx, cancel := context.WithCancelCause(t.Context())
	defer cancel(nil)
	state, started := store.Begin(&executorv1.ExecutionState{
		Id: 9, Status: executorv1.ExecutionStatus_RUNNING,
	}, cancel)
	require.False(t, started)
	require.Equal(t, executorv1.ExecutionStatus_CANCELLED, state.GetStatus())
	require.NoError(t, ctx.Err())
}
