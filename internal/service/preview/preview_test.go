package preview

import (
	"context"
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
	spec := inlineCode("print('ok')")
	resolved := domain.NewInlineProgram("print('ok')")
	runners.EXPECT().FindForExecution(gomock.Any(), int64(22)).Return(domain.Runner{
		ID: 22, Name: "Python", CodebookID: 11, Kind: domain.RunnerKindGRPC, Target: "executor",
		Handler: "python", Action: domain.RunnerActionRegistered,
		Variables: []domain.RunnerVariable{
			{Key: "REGION", Value: "default"},
			{Key: "TOKEN", Value: "secret", Secret: true},
		},
	}, nil)
	programs.EXPECT().Resolve(gomock.Any(), spec).Return(program.Resolution{Program: resolved}, nil)

	svc := &service{programSvc: programs, runnerSvc: runners}
	result, err := svc.prepare(context.Background(), RunCommand{
		RunnerID: 22,
		Program:  spec,
		Variables: []domain.RunnerVariable{
			{Key: "REGION", Value: "temporary"},
			{Key: "DEBUG", Value: "true"},
		},
	})

	require.NoError(t, err)
	require.Same(t, resolved, result.program)
	require.Equal(t, "{}", result.args)
	require.Equal(t, defaultTimeoutSeconds, result.timeout)
	require.Equal(t, []previewVariable{
		{Key: "REGION", Value: "temporary"},
		{Key: "TOKEN", Value: "secret", Secret: true},
		{Key: "DEBUG", Value: "true"},
	}, result.variables)
}

func TestPrepareRejectsRunnerFromAnotherCodebook(t *testing.T) {
	ctrl := gomock.NewController(t)
	runners := runnermocks.NewMockService(ctrl)
	runners.EXPECT().FindForExecution(gomock.Any(), int64(22)).Return(domain.Runner{
		ID: 22, CodebookID: 99, Kind: domain.RunnerKindGRPC, Target: "executor",
		Handler: "ansible", Action: domain.RunnerActionRegistered,
	}, nil)

	svc := &service{programSvc: programmocks.NewMockService(ctrl), runnerSvc: runners}
	_, err := svc.prepare(context.Background(), RunCommand{
		RunnerID: 22,
		Program: &domain.ProgramSpec{
			Kind: domain.ProgramProject, Project: &domain.ProjectProgramSpec{EntryCodebookID: 11},
		},
	})

	require.ErrorContains(t, err, "未绑定当前 Codebook")
}

func TestPrepareAcceptsProjectProgramAndAnsibleRunner(t *testing.T) {
	ctrl := gomock.NewController(t)
	programs := programmocks.NewMockService(ctrl)
	runners := runnermocks.NewMockService(ctrl)
	spec := &domain.ProgramSpec{
		Kind: domain.ProgramProject, Project: &domain.ProjectProgramSpec{EntryCodebookID: 11},
	}
	resolved := &domain.Program{Kind: domain.ProgramProject, Project: &domain.ProjectProgram{
		EntryPoint: "playbooks/site.yml",
	}}
	runners.EXPECT().FindForExecution(gomock.Any(), int64(22)).Return(domain.Runner{
		ID: 22, Name: "Ansible", CodebookID: 11, Kind: domain.RunnerKindKafka, Target: "agent-ansible",
		Handler: "ansible", Action: domain.RunnerActionRegistered,
	}, nil)
	programs.EXPECT().Resolve(gomock.Any(), spec).Return(program.Resolution{
		Program: resolved, SourceProjectID: 9,
	}, nil)

	svc := &service{programSvc: programs, runnerSvc: runners}
	result, err := svc.prepare(context.Background(), RunCommand{RunnerID: 22, Program: spec})

	require.NoError(t, err)
	require.Same(t, resolved, result.program)
	require.Equal(t, int64(9), result.sourceProjectID)
	require.Equal(t, domain.RunnerKindKafka, result.runner.Kind)
	require.Equal(t, "ansible", result.runner.Handler)
}

func TestBuildDraftUsesResolvedProgram(t *testing.T) {
	resolved := domain.NewInlineProgram("echo ok")
	draft := (&service{}).buildDraft(prepareResult{
		runner:  domain.Runner{Name: "Shell", Target: "executor", Handler: "shell"},
		program: resolved,
		args:    `{}`,
		timeout: 30,
	}, []byte(`[]`))

	require.Same(t, resolved, draft.Program)
	require.Equal(t, "试运行: Shell", draft.Task.Name)
	require.NotContains(t, draft.Task.GrpcConfig.Params, "code")
}

func inlineCode(code string) *domain.ProgramSpec {
	return &domain.ProgramSpec{Kind: domain.ProgramInline, Inline: &domain.InlineProgramSpec{Code: code}}
}
