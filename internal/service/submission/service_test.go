package submission

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/repository"
	"github.com/Duke1616/etask/internal/service/codebook"
	"github.com/Duke1616/etask/internal/service/dispatcher"
	"github.com/Duke1616/etask/internal/service/invoker"
	"github.com/Duke1616/etask/internal/service/runner"
	tasksvc "github.com/Duke1616/etask/internal/service/task"
	"github.com/Duke1616/etask/internal/service/termination"
	"github.com/stretchr/testify/require"
)

func TestValidateCommand(t *testing.T) {
	testCases := []struct {
		name    string
		command RunRunnerCommand
		wantErr string
	}{
		{name: "合法请求", command: RunRunnerCommand{RequestID: "eflow:1:1", RunnerID: 10,
			Params: map[string]string{"args": `{"ticket_id":1}`}}},
		{name: "缺少幂等标识", command: RunRunnerCommand{RunnerID: 10}, wantErr: "幂等请求标识不能为空"},
		{name: "执行单元非法", command: RunRunnerCommand{RequestID: "eflow:1:1"}, wantErr: "执行单元 ID 非法"},
		{name: "参数不是 JSON", command: RunRunnerCommand{RequestID: "eflow:1:1", RunnerID: 10,
			Params: map[string]string{"args": "{"}}, wantErr: "必须是合法 JSON"},
		{name: "空参数使用默认值", command: RunRunnerCommand{RequestID: "eflow:1:1", RunnerID: 10,
			Params: map[string]string{"args": "  "}}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			err := validateCommand(testCase.command)
			if testCase.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, testCase.wantErr)
		})
	}
}

func TestMissingExecutionPoolIsRejected(t *testing.T) {
	err := fmt.Errorf("查询执行资源池失败: %w", repository.ErrExecutionPoolNotFound)

	require.True(t, errors.Is(classifyRouteError("sd_cdc_docker_net_env_runner", err), ErrRejected))
}

func TestRunRunnerDoesNotInvokeExecutionWithEarlyCancellationIntent(t *testing.T) {
	execution := domain.TaskExecution{
		ID: 9, TenantID: 7, RequestID: "eflow:1:1", Source: domain.TaskExecutionSourceWorkflow,
		Status: domain.TaskExecutionStatusPrepare,
		Task: domain.Task{ExecMode: domain.ExecModePush, GrpcConfig: &domain.GrpcConfig{
			ServiceName: "executor", HandlerName: "shell",
		}},
		Route: domain.ExecutionRoute{
			Transport: domain.ExecutionTransportGRPC, DispatchMode: domain.ExecModePush,
			PoolName: "executor", TargetNodeID: "node-1",
		},
	}
	runs := make(chan struct{}, 1)
	service := NewService(
		&submissionRunnerStub{}, &submissionCodebookStub{},
		&submissionExecutionStub{execution: execution}, &submissionRouteStub{execution: execution},
		&submissionInvokerStub{runs: runs}, &submissionTerminationStub{},
	)

	result, err := service.RunRunner(context.Background(), RunRunnerCommand{
		RequestID: "eflow:1:1", RunnerID: 10,
	})

	require.NoError(t, err)
	require.Equal(t, domain.TaskExecutionStatusCancelled, result.Execution.Status)
	select {
	case <-runs:
		t.Fatal("命中取消意图后仍调用了执行节点")
	case <-time.After(20 * time.Millisecond):
	}
}

type submissionRunnerStub struct{ runner.Service }

func (*submissionRunnerStub) FindByID(context.Context, int64) (domain.Runner, error) {
	return domain.Runner{
		ID: 10, CodebookID: 20, Kind: domain.RunnerKindGRPC, Target: "executor",
		Handler: "shell", Action: domain.RunnerActionRegistered,
	}, nil
}

func (*submissionRunnerStub) ListMergedVariables(context.Context, int64) ([]domain.RunnerVariable, error) {
	return nil, nil
}

type submissionCodebookStub struct{ codebook.Service }

func (*submissionCodebookStub) GetByID(context.Context, int64) (domain.Codebook, error) {
	return domain.Codebook{ID: 20, ProjectID: 30, Name: "script", Kind: domain.CodebookKindFile}, nil
}

type submissionExecutionStub struct {
	tasksvc.ExecutionService
	execution domain.TaskExecution
}

func (s *submissionExecutionStub) CreateWorkflow(context.Context, domain.TaskExecution,
	int64) (domain.TaskExecution, bool, error) {
	return s.execution, true, nil
}

type submissionRouteStub struct {
	dispatcher.RoutePlanner
	execution domain.TaskExecution
}

func (s *submissionRouteStub) Plan(context.Context, domain.Task) (dispatcher.Route, error) {
	return dispatcher.Route{Task: s.execution.Task, Execution: s.execution.Route}, nil
}

type submissionTerminationStub struct{ termination.Service }

func (*submissionTerminationStub) Attach(_ context.Context,
	execution domain.TaskExecution) (domain.TaskExecution, error) {
	execution.Status = domain.TaskExecutionStatusCancelled
	return execution, nil
}

type submissionInvokerStub struct {
	invoker.Invoker
	runs chan struct{}
}

func (s *submissionInvokerStub) Run(context.Context,
	domain.TaskExecution) (domain.ExecutionState, error) {
	s.runs <- struct{}{}
	return domain.ExecutionState{}, nil
}
