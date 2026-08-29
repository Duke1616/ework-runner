package grpc

import (
	"errors"
	"testing"

	schedulerv1 "github.com/Duke1616/etask/api/proto/gen/etask/scheduler/v1"
	taskv1 "github.com/Duke1616/etask/api/proto/gen/etask/task/v1"
	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/service/submission"
	submissionmocks "github.com/Duke1616/etask/internal/service/submission/mocks"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
}

func TestSchedulerServerMapsSubmissionErrors(t *testing.T) {
	testCases := []struct {
		name     string
		result   submission.RunResult
		err      error
		wantCode codes.Code
	}{
		{name: "提交成功", result: submission.RunResult{}, wantCode: codes.OK},
		{name: "协议参数非法", err: fmtError(submission.ErrInvalidCommand), wantCode: codes.InvalidArgument},
		{name: "业务前置条件不满足", err: fmtError(submission.ErrRejected), wantCode: codes.FailedPrecondition},
		{name: "内部故障", err: errors.New("database unavailable"), wantCode: codes.Internal},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			submissions := submissionmocks.NewMockService(ctrl)
			submissions.EXPECT().RunRunner(gomock.Any(), gomock.Any()).
				Return(tc.result, tc.err)
			server := NewSchedulerServer(submissions)
			_, err := server.RunRunner(t.Context(), &schedulerv1.RunRunnerRequest{})
			require.Equal(t, tc.wantCode, status.Code(err))
		})
	}
}

func fmtError(target error) error { return errors.Join(target, errors.New("detail")) }
