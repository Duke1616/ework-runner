package runtime

import (
	"context"
	"testing"

	executorv1 "github.com/Duke1616/etask/api/proto/gen/etask/executor/v1"
	"github.com/Duke1616/etask/sdk/executor/internal/execution"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

func TestStartExecutionSkipsCancelledSchedulerExecution(t *testing.T) {
	executor := &Executor{
		executionClient: cancelledExecutionClient{},
		executions:      execution.NewStore(),
	}

	response, err := executor.startExecution(context.Background(), &executorv1.ExecuteRequest{Eid: 9})

	require.NoError(t, err)
	require.Equal(t, executorv1.ExecutionStatus_CANCELLED,
		response.GetExecutionState().GetStatus())
	stored, exists := executor.executions.Get(9)
	require.True(t, exists)
	require.Equal(t, "管理员强制结束", stored.GetTaskResult())
}

type cancelledExecutionClient struct {
	executorv1.TaskExecutionServiceClient
}

func (cancelledExecutionClient) GetTaskExecution(context.Context,
	*executorv1.GetTaskExecutionRequest, ...grpc.CallOption) (*executorv1.GetTaskExecutionResponse, error) {
	return &executorv1.GetTaskExecutionResponse{Execution: &executorv1.TaskExecution{
		Id: 9, Status: executorv1.ExecutionStatus_CANCELLED, TaskResult: "管理员强制结束",
	}}, nil
}
