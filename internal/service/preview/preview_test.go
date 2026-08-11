package preview

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Duke1616/etask/internal/domain"
	program "github.com/Duke1616/etask/internal/service/program"
	programmocks "github.com/Duke1616/etask/internal/service/program/mocks"
	runnermocks "github.com/Duke1616/etask/internal/service/runner/mocks"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestPrepareMergesTemporaryVariables(t *testing.T) {
	ctrl := gomock.NewController(t)
	programs := programmocks.NewMockService(ctrl)
	runners := runnermocks.NewMockService(ctrl)
	resolved := domain.NewInlineProgram("print('ok')")
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
	programs.EXPECT().Resolve(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, spec *domain.ProgramSpec) (program.Resolution, error) {
			require.Equal(t, domain.ProgramInline, spec.Kind)
			require.Equal(t, int64(11), spec.Inline.CodebookID)
			return program.Resolution{Program: resolved}, nil
		})

	svc := &service{programSvc: programs, runnerSvc: runners}
	result, err := svc.prepare(context.Background(), RunCommand{
		RunnerID: 22,
		Variables: []domain.RunnerVariable{
			{Key: "REGION", Value: "temporary"},
			{Key: "DEBUG", Value: "true"},
		},
	})

	require.NoError(t, err)
	require.Same(t, resolved, result.program)
	require.Equal(t, "{}", result.args)
	require.Equal(t, defaultTimeoutSeconds, result.timeout)
	require.Equal(t, []domain.RunnerVariable{
		{Key: "REGION", Value: "temporary"},
		{Key: "TOKEN", Value: "secret", Secret: true},
		{Key: "DEBUG", Value: "true"},
	}, result.variables)
}

func TestPrepareUsesAllRunnerParameterDefaults(t *testing.T) {
	ctrl := gomock.NewController(t)
	programs := programmocks.NewMockService(ctrl)
	runners := runnermocks.NewMockService(ctrl)
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
		program.Resolution{Program: domain.NewInlineProgram("ok")}, nil)

	svc := &service{programSvc: programs, runnerSvc: runners}
	result, err := svc.prepare(context.Background(), RunCommand{RunnerID: 22})

	require.NoError(t, err)
	require.Equal(t, `[{"key":"environment","value":"staging"}]`, result.params["vars"])
}

func TestPrepareAppliesGenericParameterOverrides(t *testing.T) {
	ctrl := gomock.NewController(t)
	programs := programmocks.NewMockService(ctrl)
	runners := runnermocks.NewMockService(ctrl)
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
		program.Resolution{Program: &domain.Program{Kind: domain.ProgramProject, Project: &domain.ProjectProgram{}}}, nil)

	svc := &service{programSvc: programs, runnerSvc: runners}
	result, err := svc.prepare(context.Background(), RunCommand{
		RunnerID: 22,
		Params: map[string]string{
			"args":       `{"environment":"production"}`,
			"extra_args": `--start-at-task "Deploy"`,
		},
	})

	require.NoError(t, err)
	require.JSONEq(t, `{"environment":"production"}`, result.params["args"])
	require.Equal(t, `--start-at-task "Deploy"`, result.params["extra_args"])
}

func TestPrepareRejectsRunnerWithoutProgramBinding(t *testing.T) {
	ctrl := gomock.NewController(t)
	runners := runnermocks.NewMockService(ctrl)
	runners.EXPECT().FindForExecution(gomock.Any(), int64(22)).Return(domain.RunnerExecutionSpec{
		Runner: domain.Runner{
			ID: 22, Kind: domain.RunnerKindGRPC, Target: "executor",
			Handler: "ansible", Action: domain.RunnerActionRegistered,
		},
	}, nil)

	svc := &service{programSvc: programmocks.NewMockService(ctrl), runnerSvc: runners}
	_, err := svc.prepare(context.Background(), RunCommand{
		RunnerID: 22,
	})

	require.ErrorContains(t, err, "未绑定程序来源")
}

func TestPrepareAcceptsProjectProgramAndAnsibleRunner(t *testing.T) {
	ctrl := gomock.NewController(t)
	programs := programmocks.NewMockService(ctrl)
	runners := runnermocks.NewMockService(ctrl)
	resolved := &domain.Program{Kind: domain.ProgramProject, Project: &domain.ProjectProgram{
		EntryPoint: "playbooks/site.yml",
	}}
	runners.EXPECT().FindForExecution(gomock.Any(), int64(22)).Return(domain.RunnerExecutionSpec{
		Runner: domain.Runner{
			ID: 22, Name: "Ansible", CodebookID: 11, ProgramKind: domain.ProgramProject,
			Kind: domain.RunnerKindKafka, Target: "agent-ansible",
			Handler: "ansible", Action: domain.RunnerActionRegistered,
		},
	}, nil)
	programs.EXPECT().Resolve(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, spec *domain.ProgramSpec) (program.Resolution, error) {
			require.Equal(t, domain.ProgramProject, spec.Kind)
			require.Equal(t, int64(11), spec.Project.EntryCodebookID)
			return program.Resolution{Program: resolved, SourceProjectID: 9}, nil
		})

	svc := &service{programSvc: programs, runnerSvc: runners}
	result, err := svc.prepare(context.Background(), RunCommand{RunnerID: 22})

	require.NoError(t, err)
	require.Same(t, resolved, result.program)
	require.Equal(t, int64(9), result.sourceProjectID)
	require.Equal(t, domain.RunnerKindKafka, result.runner.Kind)
	require.Equal(t, "ansible", result.runner.Handler)
}

func TestBuildDraftUsesResolvedProgram(t *testing.T) {
	resolved := domain.NewInlineProgram("echo ok")
	draft := (&service{}).buildDraft(prepareResult{
		runner:    domain.Runner{ID: 22, Name: "Shell", Target: "executor", Handler: "shell"},
		program:   resolved,
		args:      `{}`,
		timeout:   30,
		variables: []domain.RunnerVariable{{Key: "REGION", Value: "cn"}},
	})

	require.Same(t, resolved, draft.Program)
	require.Equal(t, "试运行: Shell", draft.Task.Name)
	require.NotContains(t, draft.Task.GrpcConfig.Params, "code")
	require.NotContains(t, draft.Task.GrpcConfig.Params, "variables")
	require.Equal(t, int64(22), draft.Task.RunnerID)
	require.Equal(t, []domain.RunnerVariable{{Key: "REGION", Value: "cn"}}, draft.Variables.Items)
}
