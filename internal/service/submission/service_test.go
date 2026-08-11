package submission

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/repository"
	"github.com/Duke1616/etask/internal/service/dispatcher"
	dispatchermocks "github.com/Duke1616/etask/internal/service/dispatcher/mocks"
	invokermocks "github.com/Duke1616/etask/internal/service/invoker/mocks"
	programSvc "github.com/Duke1616/etask/internal/service/program"
	programmocks "github.com/Duke1616/etask/internal/service/program/mocks"
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

func TestResolveProgramUsesRunnerCodebook(t *testing.T) {
	testCases := []struct {
		name       string
		runner     domain.Runner
		wantKind   domain.ProgramKind
		wantInline int64
		wantEntry  int64
	}{
		{name: "显式 INLINE", runner: domain.Runner{CodebookID: 20, ProgramKind: domain.ProgramInline}, wantKind: domain.ProgramInline, wantInline: 20},
		{name: "PROJECT", runner: domain.Runner{CodebookID: 20, ProgramKind: domain.ProgramProject}, wantKind: domain.ProgramProject, wantEntry: 20},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			programs := programmocks.NewMockService(ctrl)
			programs.EXPECT().Resolve(gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ context.Context, spec *domain.ProgramSpec) (programSvc.Resolution, error) {
					require.Equal(t, testCase.wantKind, spec.Kind)
					if spec.Inline != nil {
						require.Equal(t, testCase.wantInline, spec.Inline.CodebookID)
					}
					if spec.Project != nil {
						require.Equal(t, testCase.wantEntry, spec.Project.EntryCodebookID)
					}
					return programSvc.Resolution{Program: &domain.Program{Kind: testCase.wantKind}}, nil
				})
			service := &service{programs: programs}
			resolved, err := service.resolveProgram(context.Background(), testCase.runner)
			require.NoError(t, err)
			require.Equal(t, testCase.wantKind, resolved.Program.Kind)
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
	programs := programmocks.NewMockService(ctrl)
	executions := taskmocks.NewMockExecutionService(ctrl)
	routes := dispatchermocks.NewMockRoutePlanner(ctrl)
	executionInvoker := invokermocks.NewMockInvoker(ctrl)
	terminations := terminationmocks.NewMockService(ctrl)
	runner := domain.Runner{
		ID: 10, Name: "script", CodebookID: 20, ProgramKind: domain.ProgramInline,
		Kind: domain.RunnerKindGRPC, Target: "executor",
		Handler: "shell", Action: domain.RunnerActionRegistered,
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
		runners.EXPECT().FindForExecution(gomock.Any(), int64(10)).Return(
			domain.RunnerExecutionSpec{Runner: runner}, nil),
		programs.EXPECT().Resolve(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, spec *domain.ProgramSpec) (programSvc.Resolution, error) {
				require.Equal(t, domain.ProgramInline, spec.Kind)
				require.Equal(t, int64(20), spec.Inline.CodebookID)
				return programSvc.Resolution{
					Program: domain.NewInlineProgram("echo ok"), SourceProjectID: 30,
				}, nil
			}),
		routes.EXPECT().Plan(gomock.Any(), gomock.Any()).Return(dispatcher.Route{
			Task: execution.Task, Execution: execution.Route,
		}, nil),
		executions.EXPECT().CreateWorkflow(gomock.Any(), gomock.AssignableToTypeOf(domain.TaskExecution{}), int64(30)).
			DoAndReturn(func(_ context.Context, draft domain.TaskExecution, _ int64) (domain.TaskExecution, bool, error) {
				require.Equal(t, domain.ProgramInline, draft.Program.Kind)
				require.Equal(t, "echo ok", draft.Program.Inline.Code)
				require.NotContains(t, draft.Task.GrpcConfig.Params, "code")
				return execution, true, nil
			}),
		terminations.EXPECT().Attach(gomock.Any(), execution).Return(cancelled, nil),
	)
	executionInvoker.EXPECT().Run(gomock.Any(), gomock.Any()).Times(0)
	service := NewService(runners, programs, executions, routes, executionInvoker, terminations)

	result, err := service.RunRunner(context.Background(), RunRunnerCommand{
		RequestID: "eflow:1:1", RunnerID: 10,
	})

	require.NoError(t, err)
	require.Equal(t, domain.TaskExecutionStatusCancelled, result.Execution.Status)
}
