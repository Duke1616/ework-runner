package repository

import (
	"strings"
	"testing"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestTaskExecutionProgramRoundTrip(t *testing.T) {
	repository := &taskExecutionRepository{}
	execution := domain.TaskExecution{
		Program: domain.NewInlineProgram("echo ok"),
		Task:    domain.Task{GrpcConfig: &domain.GrpcConfig{HandlerName: "shell"}},
	}

	restored := repository.toDomain(repository.toEntity(execution))
	require.NotNil(t, restored.Program)
	require.Equal(t, domain.ProgramInline, restored.Program.Kind)
	require.Equal(t, "echo ok", restored.Program.Inline.Code)
}

func TestTaskExecutionProjectProgramRoundTrip(t *testing.T) {
	repository := &taskExecutionRepository{}
	execution := domain.TaskExecution{
		Program: &domain.Program{
			Kind: domain.ProgramProject,
			Project: &domain.ProjectProgram{
				Source: domain.ProjectSourceRef{
					SourceID: 11, ProjectID: 7, SourceRevision: 3,
					Digest: strings.Repeat("a", 64), BlobChecksum: strings.Repeat("b", 64),
					Size: 128, Format: "tar.zst", FormatVersion: 1,
				},
				EntryPoint: "roles/site.yml",
			},
		},
	}

	restored := repository.toDomain(repository.toEntity(execution))
	require.Equal(t, execution.Program, restored.Program)
}

func TestTaskExecutionVariablesRoundTrip(t *testing.T) {
	repository := &taskExecutionRepository{}
	execution := domain.TaskExecution{
		Task: domain.Task{RunnerID: 9},
		Variables: &domain.ExecutionVariableSet{Items: []domain.RunnerVariable{
			{Key: "region", Value: "cn"},
			{Key: "token", Value: "secret", Secret: true},
		}},
	}

	restored := repository.toDomain(repository.toEntity(execution))
	require.Equal(t, int64(9), restored.Task.RunnerID)
	require.Equal(t, execution.Variables, restored.Variables)
}
