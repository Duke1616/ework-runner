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
	"github.com/Duke1616/etask/internal/service/invoker"
	invokermocks "github.com/Duke1616/etask/internal/service/invoker/mocks"
	programSvc "github.com/Duke1616/etask/internal/service/program"
	programmocks "github.com/Duke1616/etask/internal/service/program/mocks"
	runnerSvc "github.com/Duke1616/etask/internal/service/runner"
	runnermocks "github.com/Duke1616/etask/internal/service/runner/mocks"
	taskSvc "github.com/Duke1616/etask/internal/service/task"
	taskmocks "github.com/Duke1616/etask/internal/service/task/mocks"
	terminationSvc "github.com/Duke1616/etask/internal/service/termination"
	terminationmocks "github.com/Duke1616/etask/internal/service/termination/mocks"
	"github.com/gotomicro/ego/core/elog"
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

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateCommand(tc.command)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.wantErr)
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
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			programs := programmocks.NewMockService(ctrl)
			programs.EXPECT().Resolve(gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ context.Context, spec *domain.ProgramSpec) (programSvc.Resolution, error) {
					require.Equal(t, tc.wantKind, spec.Kind)
					if spec.Inline != nil {
						require.Equal(t, tc.wantInline, spec.Inline.CodebookID)
					}
					if spec.Project != nil {
						require.Equal(t, tc.wantEntry, spec.Project.EntryCodebookID)
					}
					return programSvc.Resolution{Program: &domain.Program{Kind: tc.wantKind}}, nil
				})
			service := &service{programs: programs}
			resolved, err := service.resolveProgram(context.Background(), tc.runner)
			require.NoError(t, err)
			require.Equal(t, tc.wantKind, resolved.Program.Kind)
		})
	}
}

func TestClassifyRouteError(t *testing.T) {
	testCases := []struct {
		name      string
		target    string
		err       error
		wantErrIs error
		wantErr   string
	}{
		{
			name:      "资源池不存在转为拒绝错误",
			target:    "k8s_pool",
			err:       fmt.Errorf("not found: %w", repository.ErrExecutionPoolNotFound),
			wantErrIs: ErrRejected,
			wantErr:   "执行资源池 \"k8s_pool\" 不存在",
		},
		{
			name:    "其他规划错误保留原始信息",
			target:  "default_pool",
			err:     errors.New("db disconnect"),
			wantErr: "规划工作流执行路由失败: db disconnect",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotErr := classifyRouteError(tc.target, tc.err)
			if tc.wantErrIs != nil {
				require.ErrorIs(t, gotErr, tc.wantErrIs)
			}
			require.ErrorContains(t, gotErr, tc.wantErr)
		})
	}
}

func TestService_BuildDraft(t *testing.T) {
	testCases := []struct {
		name      string
		command   RunRunnerCommand
		runner    domain.Runner
		program   *domain.Program
		params    map[string]string
		variables []domain.RunnerVariable
		validate  func(t *testing.T, draft domain.TaskExecution)
	}{
		{
			name: "成功构建草稿并深拷贝隔离",
			command: RunRunnerCommand{
				RequestID: "req:1001",
				RunnerID:  10,
			},
			runner: domain.Runner{
				ID:      10,
				Name:    "Ansible执行单元",
				Target:  "executor-pool",
				Handler: "ansible",
			},
			program: domain.NewInlineProgram("echo ok"),
			params: map[string]string{
				"args": "{}",
				"env":  "staging",
			},
			variables: []domain.RunnerVariable{
				{Key: "TOKEN", Value: "secret", Secret: true},
			},
			validate: func(t *testing.T, draft domain.TaskExecution) {
				require.Equal(t, "req:1001", draft.RequestID)
				require.Equal(t, "工作流执行: Ansible执行单元", draft.Task.Name)
				require.Equal(t, int64(10), draft.Task.RunnerID)
				require.Equal(t, defaultTimeoutSeconds, draft.Task.MaxExecutionSeconds)
				require.Equal(t, "executor-pool", draft.Task.GrpcConfig.ServiceName)
				require.Equal(t, "ansible", draft.Task.GrpcConfig.HandlerName)
				require.Equal(t, map[string]string{"args": "{}", "env": "staging"}, draft.Task.GrpcConfig.Params)
				require.Equal(t, []domain.RunnerVariable{{Key: "TOKEN", Value: "secret", Secret: true}}, draft.Variables.Items)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &service{}
			draft := svc.buildDraft(tc.command, tc.runner, tc.program, tc.params, tc.variables)
			tc.validate(t, draft)
		})
	}
}

func TestService_RunRunner(t *testing.T) {
	testCases := []struct {
		name      string
		command   RunRunnerCommand
		setupMock func(ctrl *gomock.Controller) (runnerSvc.Service, programSvc.Service, taskSvc.ExecutionService,
			dispatcher.RoutePlanner, invoker.Invoker, terminationSvc.Service)
		wantErrIs error
		wantErr   string
		validate  func(t *testing.T, result RunResult)
	}{
		{
			name: "执行单元未启用拒绝执行",
			command: RunRunnerCommand{
				RequestID: "eflow:1:1",
				RunnerID:  10,
			},
			setupMock: func(ctrl *gomock.Controller) (runnerSvc.Service, programSvc.Service, taskSvc.ExecutionService,
				dispatcher.RoutePlanner, invoker.Invoker, terminationSvc.Service) {
				runners := runnermocks.NewMockService(ctrl)
				runners.EXPECT().FindForExecution(gomock.Any(), int64(10)).Return(
					domain.RunnerExecutionSpec{
						Runner: domain.Runner{ID: 10, Action: domain.RunnerActionUnregistered},
					}, nil)
				return runners, nil, nil, nil, nil, nil
			},
			wantErrIs: ErrRejected,
			wantErr:   "执行单元未启用",
		},
		{
			name: "通道不匹配被拒绝执行",
			command: RunRunnerCommand{
				RequestID: "eflow:1:1",
				RunnerID:  10,
			},
			setupMock: func(ctrl *gomock.Controller) (runnerSvc.Service, programSvc.Service, taskSvc.ExecutionService,
				dispatcher.RoutePlanner, invoker.Invoker, terminationSvc.Service) {
				runners := runnermocks.NewMockService(ctrl)
				programs := programmocks.NewMockService(ctrl)
				routes := dispatchermocks.NewMockRoutePlanner(ctrl)
				runners.EXPECT().FindForExecution(gomock.Any(), int64(10)).Return(
					domain.RunnerExecutionSpec{
						Runner: domain.Runner{
							ID: 10, Action: domain.RunnerActionRegistered,
							CodebookID: 20, ProgramKind: domain.ProgramInline,
							Kind: domain.RunnerKindGRPC, Target: "executor", Handler: "shell",
						},
					}, nil)
				programs.EXPECT().Resolve(gomock.Any(), gomock.Any()).Return(
					programSvc.Resolution{Program: domain.NewInlineProgram("echo ok")}, nil)
				routes.EXPECT().Plan(gomock.Any(), gomock.Any()).Return(
					dispatcher.Route{
						Execution: domain.ExecutionRoute{Transport: domain.ExecutionTransportMQ},
					}, nil)
				return runners, programs, nil, routes, nil, nil
			},
			wantErrIs: ErrRejected,
			wantErr:   "与资源池传输通道",
		},
		{
			name: "成功执行_附带提前取消意图不派发执行器",
			command: RunRunnerCommand{
				RequestID: "eflow:1:1",
				RunnerID:  10,
			},
			setupMock: func(ctrl *gomock.Controller) (runnerSvc.Service, programSvc.Service, taskSvc.ExecutionService,
				dispatcher.RoutePlanner, invoker.Invoker, terminationSvc.Service) {
				runners := runnermocks.NewMockService(ctrl)
				programs := programmocks.NewMockService(ctrl)
				executions := taskmocks.NewMockExecutionService(ctrl)
				routes := dispatchermocks.NewMockRoutePlanner(ctrl)
				inv := invokermocks.NewMockInvoker(ctrl)
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

				runners.EXPECT().FindForExecution(gomock.Any(), int64(10)).Return(
					domain.RunnerExecutionSpec{Runner: runner}, nil)
				programs.EXPECT().Resolve(gomock.Any(), gomock.Any()).Return(
					programSvc.Resolution{Program: domain.NewInlineProgram("echo ok"), SourceProjectID: 30}, nil)
				routes.EXPECT().Plan(gomock.Any(), gomock.Any()).Return(
					dispatcher.Route{Task: execution.Task, Execution: execution.Route}, nil)
				executions.EXPECT().CreateWorkflow(gomock.Any(), gomock.Any(), int64(30)).Return(execution, true, nil)
				terminations.EXPECT().Attach(gomock.Any(), execution).Return(cancelled, nil)
				inv.EXPECT().Run(gomock.Any(), gomock.Any()).Times(0)

				return runners, programs, executions, routes, inv, terminations
			},
			validate: func(t *testing.T, result RunResult) {
				require.True(t, result.Created)
				require.Equal(t, domain.TaskExecutionStatusCancelled, result.Execution.Status)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			runners, programs, executions, routes, inv, terminations := tc.setupMock(ctrl)
			svc := NewService(runners, programs, executions, routes, inv, terminations)
			res, err := svc.RunRunner(t.Context(), tc.command)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				if tc.wantErrIs != nil {
					require.ErrorIs(t, err, tc.wantErrIs)
				}
				return
			}
			require.NoError(t, err)
			tc.validate(t, res)
		})
	}
}

func TestService_TerminateExecution(t *testing.T) {
	testCases := []struct {
		name      string
		command   TerminateExecutionCommand
		setupMock func(ctrl *gomock.Controller) terminationSvc.Service
		wantErrIs error
		wantErr   string
	}{
		{
			name: "成功申请终止",
			command: TerminateExecutionCommand{
				ExecutionID: 100, RequestID: "req:100", Reason: "用户取消",
			},
			setupMock: func(ctrl *gomock.Controller) terminationSvc.Service {
				mock := terminationmocks.NewMockService(ctrl)
				mock.EXPECT().Request(gomock.Any(), terminationSvc.Request{
					ExecutionID: 100, RequestID: "req:100", Reason: "用户取消",
				}).Return(nil)
				return mock
			},
		},
		{
			name: "参数非法被拒绝",
			command: TerminateExecutionCommand{ExecutionID: 0},
			setupMock: func(ctrl *gomock.Controller) terminationSvc.Service {
				mock := terminationmocks.NewMockService(ctrl)
				mock.EXPECT().Request(gomock.Any(), gomock.Any()).Return(terminationSvc.ErrInvalidCommand)
				return mock
			},
			wantErrIs: ErrInvalidCommand,
		},
		{
			name: "终止被拒绝",
			command: TerminateExecutionCommand{ExecutionID: 100},
			setupMock: func(ctrl *gomock.Controller) terminationSvc.Service {
				mock := terminationmocks.NewMockService(ctrl)
				mock.EXPECT().Request(gomock.Any(), gomock.Any()).Return(terminationSvc.ErrRejected)
				return mock
			},
			wantErrIs: ErrRejected,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			term := tc.setupMock(ctrl)
			svc := &service{termination: term}
			err := svc.TerminateExecution(t.Context(), tc.command)
			if tc.wantErrIs != nil {
				require.ErrorIs(t, err, tc.wantErrIs)
			}
		})
	}
}

func TestService_Invoke(t *testing.T) {
	testCases := []struct {
		name      string
		mock      func(ctrl *gomock.Controller) (invoker.Invoker, taskSvc.ExecutionService)
		execution domain.TaskExecution
	}{
		{
			name: "panic崩溃安全恢复并置为失败",
			mock: func(ctrl *gomock.Controller) (invoker.Invoker, taskSvc.ExecutionService) {
				inv := invokermocks.NewMockInvoker(ctrl)
				exec := taskmocks.NewMockExecutionService(ctrl)
				inv.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, _ domain.TaskExecution) (domain.ExecutionState, error) {
						panic("simulated runtime panic in submission invoker")
					})
				exec.EXPECT().UpdateState(gomock.Any(), gomock.Cond(func(x any) bool {
					state, ok := x.(domain.ExecutionState)
					return ok && state.ID == 88 && state.Status == domain.TaskExecutionStatusFailed
				})).Return(nil)
				return inv, exec
			},
			execution: domain.TaskExecution{
				ID:   88,
				Task: domain.Task{ID: 10, Name: "测试工作流任务"},
			},
		},
		{
			name: "调用执行器错误安全记录并置为失败",
			mock: func(ctrl *gomock.Controller) (invoker.Invoker, taskSvc.ExecutionService) {
				inv := invokermocks.NewMockInvoker(ctrl)
				exec := taskmocks.NewMockExecutionService(ctrl)
				inv.EXPECT().Run(gomock.Any(), gomock.Any()).Return(
					domain.ExecutionState{}, errors.New("connection reset"))
				exec.EXPECT().UpdateState(gomock.Any(), gomock.Cond(func(x any) bool {
					state, ok := x.(domain.ExecutionState)
					return ok && state.ID == 89 && state.Status == domain.TaskExecutionStatusFailed
				})).Return(nil)
				return inv, exec
			},
			execution: domain.TaskExecution{
				ID:   89,
				Task: domain.Task{ID: 11, Name: "测试连接重置"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			inv, exec := tc.mock(ctrl)
			svc := &service{invoker: inv, executions: exec, logger: elog.DefaultLogger}
			require.NotPanics(t, func() {
				svc.invoke(t.Context(), tc.execution)
			})
		})
	}
}
