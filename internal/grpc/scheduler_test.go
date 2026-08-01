package grpc

import (
	"context"
	"errors"
	"testing"

	schedulerv1 "github.com/Duke1616/etask/api/proto/gen/etask/scheduler/v1"
	"github.com/Duke1616/etask/internal/service/submission"
	"github.com/stretchr/testify/require"
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
			server := NewSchedulerServer(&submissionServiceStub{result: testCase.result, err: testCase.err})
			_, err := server.RunRunner(context.Background(), &schedulerv1.RunRunnerRequest{})
			require.Equal(t, testCase.wantCode, status.Code(err))
		})
	}
}

func fmtError(target error) error { return errors.Join(target, errors.New("detail")) }

type submissionServiceStub struct {
	result submission.RunResult
	err    error
}

func (s *submissionServiceStub) RunRunner(context.Context,
	submission.RunRunnerCommand) (submission.RunResult, error) {
	return s.result, s.err
}

func (s *submissionServiceStub) TerminateExecution(context.Context,
	submission.TerminateExecutionCommand) error {
	return s.err
}
