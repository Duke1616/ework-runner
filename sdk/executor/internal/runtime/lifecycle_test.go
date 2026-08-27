package runtime

import (
	"context"
	"testing"
	"time"

	executorv1 "github.com/Duke1616/etask/api/proto/gen/etask/executor/v1"
	grpcpkg "github.com/Duke1616/etask/pkg/grpc"
	"github.com/Duke1616/etask/sdk/executor/internal/execution"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestPullLoopFollowsServerLifecycle(t *testing.T) {
	executor, err := NewExecutor(Config{
		Mode: ModePull,
		Server: grpcpkg.ServerConfig{
			ServiceId: "node-1", ServiceName: "executor", ListenAddr: "127.0.0.1:0",
		},
		Client: grpcpkg.ClientConfig{Name: "scheduler"},
	}, registryStub{})
	require.NoError(t, err)
	require.NoError(t, executor.InitComponents())
	require.Nil(t, executor.pullCancel)

	require.NoError(t, executor.Start())
	require.NotNil(t, executor.pullCancel)
	require.NoError(t, executor.Stop())
	require.Nil(t, executor.pullCancel)
}

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

func TestGracefulStopCancelsRunningExecutions(t *testing.T) {
	executor := &Executor{executions: execution.NewStore()}
	runCtx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	_, started := executor.executions.Begin(&executorv1.ExecutionState{
		Id: 1, Status: executorv1.ExecutionStatus_RUNNING,
	}, cancel)
	require.True(t, started)

	executor.runWG.Add(1)
	go func() {
		defer executor.runWG.Done()
		<-runCtx.Done()
	}()

	require.NoError(t, executor.GracefulStop(t.Context()))
	require.ErrorIs(t, context.Cause(runCtx), execution.ErrInterrupted)
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

func TestConnectionClosingError(t *testing.T) {
	require.True(t, isConnectionClosingError(status.Error(codes.Canceled, "client connection is closing")))
	require.False(t, isConnectionClosingError(status.Error(codes.Unavailable, "scheduler unavailable")))
}
