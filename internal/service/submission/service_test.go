package submission

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/repository"
	codebookmocks "github.com/Duke1616/etask/internal/service/codebook/mocks"
	"github.com/Duke1616/etask/internal/service/dispatcher"
	dispatchermocks "github.com/Duke1616/etask/internal/service/dispatcher/mocks"
	invokermocks "github.com/Duke1616/etask/internal/service/invoker/mocks"
	runnermocks "github.com/Duke1616/etask/internal/service/runner/mocks"
	taskmocks "github.com/Duke1616/etask/internal/service/task/mocks"
	terminationmocks "github.com/Duke1616/etask/internal/service/termination/mocks"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
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
	ctrl := gomock.NewController(t)
	runners := runnermocks.NewMockService(ctrl)
	codebooks := codebookmocks.NewMockService(ctrl)
	executions := taskmocks.NewMockExecutionService(ctrl)
	routes := dispatchermocks.NewMockRoutePlanner(ctrl)
	executionInvoker := invokermocks.NewMockInvoker(ctrl)
	terminations := terminationmocks.NewMockService(ctrl)
	runner := domain.Runner{
		ID: 10, CodebookID: 20, Kind: domain.RunnerKindGRPC, Target: "executor",
		Handler: "shell", Action: domain.RunnerActionRegistered,
	}
	codebook := domain.Codebook{
		ID: 20, ProjectID: 30, Name: "script", Kind: domain.CodebookKindFile,
	}
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
	cancelled := execution
	cancelled.Status = domain.TaskExecutionStatusCancelled
	gomock.InOrder(
		runners.EXPECT().FindByID(gomock.Any(), int64(10)).Return(runner, nil),
		codebooks.EXPECT().GetByID(gomock.Any(), int64(20)).Return(codebook, nil),
		runners.EXPECT().ListMergedVariables(gomock.Any(), int64(10)).Return(nil, nil),
		routes.EXPECT().Plan(gomock.Any(), gomock.Any()).Return(dispatcher.Route{
			Task: execution.Task, Execution: execution.Route,
		}, nil),
		executions.EXPECT().CreateWorkflow(gomock.Any(), gomock.Any(), int64(30)).
			Return(execution, true, nil),
		terminations.EXPECT().Attach(gomock.Any(), execution).Return(cancelled, nil),
	)
	executionInvoker.EXPECT().Run(gomock.Any(), gomock.Any()).Times(0)
	service := NewService(runners, codebooks, executions, routes, executionInvoker, terminations)

	result, err := service.RunRunner(context.Background(), RunRunnerCommand{
		RequestID: "eflow:1:1", RunnerID: 10,
	})

	require.NoError(t, err)
	require.Equal(t, domain.TaskExecutionStatusCancelled, result.Execution.Status)
}
