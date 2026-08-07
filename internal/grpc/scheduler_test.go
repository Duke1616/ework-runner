package grpc

import (
	"context"
	"errors"
	"testing"

	schedulerv1 "github.com/Duke1616/etask/api/proto/gen/etask/scheduler/v1"
	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/service/submission"
	submissionmocks "github.com/Duke1616/etask/internal/service/submission/mocks"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

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

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			submissions := submissionmocks.NewMockService(ctrl)
			submissions.EXPECT().RunRunner(gomock.Any(), gomock.Any()).
				Return(testCase.result, testCase.err)
			server := NewSchedulerServer(submissions)
			_, err := server.RunRunner(context.Background(), &schedulerv1.RunRunnerRequest{})
			require.Equal(t, testCase.wantCode, status.Code(err))
		})
	}
}

func TestSchedulerProgramKindMapping(t *testing.T) {
	require.Equal(t, domain.ProgramInline,
		toDomainProgramKind(schedulerv1.ProgramKind_PROGRAM_KIND_UNSPECIFIED))
	require.Equal(t, domain.ProgramInline,
		toDomainProgramKind(schedulerv1.ProgramKind_PROGRAM_KIND_INLINE))
	require.Equal(t, domain.ProgramProject,
		toDomainProgramKind(schedulerv1.ProgramKind_PROGRAM_KIND_PROJECT))
	require.False(t, toDomainProgramKind(schedulerv1.ProgramKind(99)).Valid())
}

func fmtError(target error) error { return errors.Join(target, errors.New("detail")) }
