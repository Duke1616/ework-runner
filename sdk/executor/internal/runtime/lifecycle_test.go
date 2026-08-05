package runtime

import (
	"context"
	"testing"
	"time"

	grpcpkg "github.com/Duke1616/etask/pkg/grpc"
	"github.com/Duke1616/etask/sdk/executor/internal/execution"
	"github.com/stretchr/testify/require"
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

func TestGracefulStopHonorsContext(t *testing.T) {
	executor := &Executor{executions: execution.NewStore()}
	executor.runWG.Add(1)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()

	err := executor.GracefulStop(ctx)
	executor.runWG.Done()
	require.ErrorIs(t, err, context.DeadlineExceeded)
}
