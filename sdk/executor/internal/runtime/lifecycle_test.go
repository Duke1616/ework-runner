package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/Duke1616/etask/sdk/executor/internal/execution"
	"github.com/stretchr/testify/require"
)

func TestGracefulStopWaitsForBackgroundExecutions(t *testing.T) {
	executor := &Executor{executions: execution.NewStore()}
	executor.runWG.Add(1)
	released := make(chan struct{})
	go func() {
		<-released
		executor.runWG.Done()
	}()
	go func() {
		time.Sleep(10 * time.Millisecond)
		close(released)
	}()

	require.NoError(t, executor.GracefulStop(t.Context()))
	require.True(t, executor.stopping)
}

func TestGracefulStopHonorsContext(t *testing.T) {
	executor := &Executor{executions: execution.NewStore()}
	executor.runWG.Add(1)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()

	err := executor.GracefulStop(ctx)
	executor.runWG.Done()
	require.ErrorIs(t, err, context.DeadlineExceeded)
}
