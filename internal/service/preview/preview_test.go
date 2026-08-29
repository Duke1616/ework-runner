package preview

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/service/invoker"
	invokermocks "github.com/Duke1616/etask/internal/service/invoker/mocks"
	program "github.com/Duke1616/etask/internal/service/program"
	programmocks "github.com/Duke1616/etask/internal/service/program/mocks"
	runnerSvc "github.com/Duke1616/etask/internal/service/runner"
	runnermocks "github.com/Duke1616/etask/internal/service/runner/mocks"
	taskSvc "github.com/Duke1616/etask/internal/service/task"
	taskmocks "github.com/Duke1616/etask/internal/service/task/mocks"
	"github.com/gotomicro/ego/core/elog"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestService_Prepare(t *testing.T) {
	testCases := []struct {
		name    string
		mock    func(ctrl *gomock.Controller, resolved *domain.Program) (runnerSvc.Service, program.Service)
		command RunCommand
		want    func(t *testing.T, res prepareResult, err error, resolved *domain.Program)
	}{
		{
			name: "成功_合并执行单元默认参数与临时变量",
			mock: func(ctrl *gomock.Controller, resolved *domain.Program) (runnerSvc.Service, program.Service) {
				runners := runnermocks.NewMockService(ctrl)
				programs := programmocks.NewMockService(ctrl)
				runners.EXPECT().FindForExecution(gomock.Any(), int64(22)).Return(domain.RunnerExecutionSpec{
					Runner: domain.Runner{
						ID: 22, Name: "Python", CodebookID: 11, ProgramKind: domain.ProgramInline,
						Kind: domain.RunnerKindGRPC, Target: "executor",
						Handler: "python", Action: domain.RunnerActionRegistered,
					},
					Variables: []domain.RunnerVariable{
						{Key: "REGION", Value: "default"},
						{Key: "TOKEN", Value: "secret", Secret: true},
					},
				}, nil)
				programs.EXPECT().Resolve(gomock.Any(), gomock.Any()).Return(
					program.Resolution{Program: resolved}, nil)
				return runners, programs
			},
			command: RunCommand{
				RunnerID: 22,
				Variables: []domain.RunnerVariable{
					{Key: "REGION", Value: "temporary"},
					{Key: "DEBUG", Value: "true"},
				},
			},
			want: func(t *testing.T, res prepareResult, err error, resolved *domain.Program) {
				require.NoError(t, err)
				require.Same(t, resolved, res.program)
				require.Equal(t, "{}", res.args)
				require.Equal(t, defaultTimeoutSeconds, res.timeout)
				require.Equal(t, []domain.RunnerVariable{
					{Key: "REGION", Value: "temporary"},
					{Key: "TOKEN", Value: "secret", Secret: true},
					{Key: "DEBUG", Value: "true"},
				}, res.variables)
			},
		},
		{
			name: "成功_应用执行单元所有默认参数",
			mock: func(ctrl *gomock.Controller, resolved *domain.Program) (runnerSvc.Service, program.Service) {
				runners := runnermocks.NewMockService(ctrl)
				programs := programmocks.NewMockService(ctrl)
				runners.EXPECT().FindForExecution(gomock.Any(), int64(22)).Return(domain.RunnerExecutionSpec{
					Runner: domain.Runner{
						ID: 22, CodebookID: 11, ProgramKind: domain.ProgramInline,
						Kind: domain.RunnerKindGRPC, Target: "executor", Handler: "ansible",
						Action: domain.RunnerActionRegistered,
						ParameterDefaults: map[string]json.RawMessage{
							"vars": json.RawMessage(`[{"key":"environment","value":"staging"}]`),
						},
					},
				}, nil)
				programs.EXPECT().Resolve(gomock.Any(), gomock.Any()).Return(
					program.Resolution{Program: resolved}, nil)
				return runners, programs
			},
			command: RunCommand{RunnerID: 22},
			want: func(t *testing.T, res prepareResult, err error, _ *domain.Program) {
				require.NoError(t, err)
				require.Equal(t, `[{"key":"environment","value":"staging"}]`, res.params["vars"])
			},
		},
		{
			name: "成功_入参覆盖默认参数",
			mock: func(ctrl *gomock.Controller, resolved *domain.Program) (runnerSvc.Service, program.Service) {
				runners := runnermocks.NewMockService(ctrl)
				programs := programmocks.NewMockService(ctrl)
				runners.EXPECT().FindForExecution(gomock.Any(), int64(22)).Return(domain.RunnerExecutionSpec{
					Runner: domain.Runner{
						ID: 22, CodebookID: 11, ProgramKind: domain.ProgramProject,
						Kind: domain.RunnerKindGRPC, Target: "executor", Handler: "ansible",
						Action: domain.RunnerActionRegistered,
						ParameterDefaults: map[string]json.RawMessage{
							"args":       json.RawMessage(`{"environment":"staging"}`),
							"extra_args": json.RawMessage(`"--syntax-check"`),
						},
					},
				}, nil)
				programs.EXPECT().Resolve(gomock.Any(), gomock.Any()).Return(
					program.Resolution{Program: resolved}, nil)
				return runners, programs
			},
			command: RunCommand{
				RunnerID: 22,
				Params: map[string]string{
					"args":       `{"environment":"production"}`,
					"extra_args": `--start-at-task "Deploy"`,
				},
			},
			want: func(t *testing.T, res prepareResult, err error, _ *domain.Program) {
				require.NoError(t, err)
				require.JSONEq(t, `{"environment":"production"}`, res.params["args"])
				require.Equal(t, `--start-at-task "Deploy"`, res.params["extra_args"])
			},
		},
		{
			name: "成功_PROJECT程序与Kafka通道",
			mock: func(ctrl *gomock.Controller, resolved *domain.Program) (runnerSvc.Service, program.Service) {
				runners := runnermocks.NewMockService(ctrl)
				programs := programmocks.NewMockService(ctrl)
				runners.EXPECT().FindForExecution(gomock.Any(), int64(22)).Return(domain.RunnerExecutionSpec{
					Runner: domain.Runner{
						ID: 22, Name: "Ansible", CodebookID: 11, ProgramKind: domain.ProgramProject,
						Kind: domain.RunnerKindKafka, Target: "agent-ansible",
						Handler: "ansible", Action: domain.RunnerActionRegistered,
					},
				}, nil)
				programs.EXPECT().Resolve(gomock.Any(), gomock.Any()).Return(
					program.Resolution{Program: resolved, SourceProjectID: 9}, nil)
				return runners, programs
			},
			command: RunCommand{RunnerID: 22},
			want: func(t *testing.T, res prepareResult, err error, resolved *domain.Program) {
				require.NoError(t, err)
				require.Same(t, resolved, res.program)
				require.Equal(t, int64(9), res.sourceProjectID)
				require.Equal(t, domain.RunnerKindKafka, res.runner.Kind)
				require.Equal(t, "ansible", res.runner.Handler)
			},
		},
		{
			name: "失败_Runner未绑定代码本",
			mock: func(ctrl *gomock.Controller, _ *domain.Program) (runnerSvc.Service, program.Service) {
				runners := runnermocks.NewMockService(ctrl)
				runners.EXPECT().FindForExecution(gomock.Any(), int64(22)).Return(domain.RunnerExecutionSpec{
					Runner: domain.Runner{
						ID: 22, Kind: domain.RunnerKindGRPC, Target: "executor",
						Handler: "ansible", Action: domain.RunnerActionRegistered,
					},
				}, nil)
				return runners, programmocks.NewMockService(ctrl)
			},
			command: RunCommand{RunnerID: 22},
			want: func(t *testing.T, _ prepareResult, err error, _ *domain.Program) {
				require.ErrorContains(t, err, "未绑定程序来源")
			},
		},
		{
			name: "失败_RunnerID非法",
			mock: func(ctrl *gomock.Controller, _ *domain.Program) (runnerSvc.Service, program.Service) {
				return runnermocks.NewMockService(ctrl), programmocks.NewMockService(ctrl)
			},
			command: RunCommand{RunnerID: 0},
			want: func(t *testing.T, _ prepareResult, err error, _ *domain.Program) {
				require.ErrorContains(t, err, "必须大于 0")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			resolved := domain.NewInlineProgram("print('ok')")
			runnerMock, programMock := tc.mock(ctrl, resolved)
			svc := &service{programSvc: programMock, runnerSvc: runnerMock}
			res, err := svc.prepare(t.Context(), tc.command)
			tc.want(t, res, err, resolved)
		})
	}
}

func TestService_BuildDraft(t *testing.T) {
	testCases := []struct {
		name     string
		input    prepareResult
		validate func(t *testing.T, draft domain.TaskExecution)
	}{
		{
			name: "标准草稿组装与深拷贝隔离",
			input: prepareResult{
				runner:    domain.Runner{ID: 22, Name: "Shell", Target: "executor", Handler: "shell"},
				program:   domain.NewInlineProgram("echo ok"),
				args:      "{}",
				params:    map[string]string{"args": "{}", "env": "prod"},
				timeout:   30,
				variables: []domain.RunnerVariable{{Key: "REGION", Value: "cn"}},
			},
			validate: func(t *testing.T, draft domain.TaskExecution) {
				require.Equal(t, "试运行: Shell", draft.Task.Name)
				require.Equal(t, int64(22), draft.Task.RunnerID)
				require.Equal(t, int64(30), draft.Task.MaxExecutionSeconds)
				require.Equal(t, "executor", draft.Task.GrpcConfig.ServiceName)
				require.Equal(t, "shell", draft.Task.GrpcConfig.HandlerName)
				require.Equal(t, map[string]string{"args": "{}", "env": "prod"}, draft.Task.GrpcConfig.Params)
				require.Equal(t, []domain.RunnerVariable{{Key: "REGION", Value: "cn"}}, draft.Variables.Items)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			draft := (&service{}).buildDraft(tc.input)
			tc.validate(t, draft)
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
						panic("simulated runtime panic in invoker")
					})
				exec.EXPECT().UpdateState(gomock.Any(), gomock.Cond(func(x any) bool {
					state, ok := x.(domain.ExecutionState)
					return ok && state.ID == 99 && state.Status == domain.TaskExecutionStatusFailed
				})).Return(nil)
				return inv, exec
			},
			execution: domain.TaskExecution{
				ID:   99,
				Task: domain.Task{ID: 101, Name: "测试试运行"},
			},
		},
		{
			name: "执行器调用失败置为失败",
			mock: func(ctrl *gomock.Controller) (invoker.Invoker, taskSvc.ExecutionService) {
				inv := invokermocks.NewMockInvoker(ctrl)
				exec := taskmocks.NewMockExecutionService(ctrl)
				inv.EXPECT().Run(gomock.Any(), gomock.Any()).Return(
					domain.ExecutionState{}, errors.New("rpc error"))
				exec.EXPECT().UpdateState(gomock.Any(), gomock.Cond(func(x any) bool {
					state, ok := x.(domain.ExecutionState)
					return ok && state.ID == 100 && state.Status == domain.TaskExecutionStatusFailed
				})).Return(nil)
				return inv, exec
			},
			execution: domain.TaskExecution{
				ID:   100,
				Task: domain.Task{ID: 102, Name: "测试调用错误"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			inv, exec := tc.mock(ctrl)
			svc := &service{invoker: inv, execSvc: exec, logger: elog.DefaultLogger}
			require.NotPanics(t, func() {
				svc.invoke(t.Context(), tc.execution)
			})
		})
	}
}
