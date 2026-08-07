package grpc

import (
	"testing"

	taskv1 "github.com/Duke1616/etask/api/proto/gen/etask/task/v1"
	"github.com/Duke1616/etask/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestTaskProgramSpecConversion(t *testing.T) {
	testCases := []struct {
		name string
		spec *taskv1.ProgramSpec
		want *domain.ProgramSpec
	}{
		{
			name: "INLINE 直接代码",
			spec: &taskv1.ProgramSpec{Source: &taskv1.ProgramSpec_Inline{Inline: &taskv1.InlineProgramSpec{
				Source: &taskv1.InlineProgramSpec_Code{Code: "echo ok"},
			}}},
			want: &domain.ProgramSpec{Kind: domain.ProgramInline, Inline: &domain.InlineProgramSpec{Code: "echo ok"}},
		},
		{
			name: "INLINE Codebook",
			spec: &taskv1.ProgramSpec{Source: &taskv1.ProgramSpec_Inline{Inline: &taskv1.InlineProgramSpec{
				Source: &taskv1.InlineProgramSpec_CodebookId{CodebookId: 11},
			}}},
			want: &domain.ProgramSpec{Kind: domain.ProgramInline, Inline: &domain.InlineProgramSpec{CodebookID: 11}},
		},
		{
			name: "PROJECT",
			spec: &taskv1.ProgramSpec{Source: &taskv1.ProgramSpec_Project{Project: &taskv1.ProjectProgramSpec{
				EntryCodebookId: 11,
			}}},
			want: &domain.ProgramSpec{Kind: domain.ProgramProject, Project: &domain.ProjectProgramSpec{EntryCodebookID: 11}},
		},
	}

	server := &TaskServer{}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			created := server.toDomainTask(7, &taskv1.CreateTaskRequest{
				ExecMode: taskv1.ExecMode_PULL, Program: tc.spec,
				GrpcConfig: &taskv1.GrpcConfig{HandlerName: "shell", Params: map[string]string{"args": `{}`}},
			})
			require.Equal(t, tc.want, created.Program)
			require.NotContains(t, created.GrpcConfig.Params, "code")
			converted := server.toProtoTask(created)
			require.Equal(t, tc.spec, converted.GetProgram())
			require.Equal(t, taskv1.ExecMode_PULL, converted.GetExecMode())
		})
	}
}

func TestTaskWithoutProgramRemainsProgramless(t *testing.T) {
	server := &TaskServer{}
	task := server.toDomainTask(7, &taskv1.CreateTaskRequest{
		GrpcConfig: &taskv1.GrpcConfig{HandlerName: "demo"},
	})
	require.Nil(t, task.Program)
	require.Nil(t, server.toProtoTask(task).GetProgram())
}
