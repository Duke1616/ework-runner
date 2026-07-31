package grpc_test

import (
	"context"
	"errors"
	"testing"

	executorv1 "github.com/Duke1616/etask/api/proto/gen/etask/executor/v1"
	reporterv1 "github.com/Duke1616/etask/api/proto/gen/etask/reporter/v1"
	"github.com/Duke1616/etask/internal/domain"
	etaskgrpc "github.com/Duke1616/etask/internal/grpc"
	grpcmocks "github.com/Duke1616/etask/internal/grpc/mocks"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestReporterSeparatesLogsAndState(t *testing.T) {
	t.Run("仅日志上报不更新状态", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		executions := grpcmocks.NewMockExecutionReportHandler(ctrl)
		executions.EXPECT().AppendExecutionLogs(gomock.Any(), int64(10), int64(20), []string{"日志"}).Return(nil)
		server := etaskgrpc.NewReporterServer(executions)

		_, err := server.Report(context.Background(), &reporterv1.ReportRequest{
			ExecutionState: &executorv1.ExecutionState{Id: 10, TaskId: 20},
			LogChunks:      []string{"日志"},
			LogOnly:        true,
		})
		require.NoError(t, err)
	})

	t.Run("终态上报进入状态机", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		executions := grpcmocks.NewMockExecutionReportHandler(ctrl)
		state := domain.ExecutionState{ID: 10, TaskID: 20, Status: domain.TaskExecutionStatusSuccess}
		executions.EXPECT().AppendExecutionLogs(gomock.Any(), int64(10), int64(20), []string(nil)).Return(nil)
		executions.EXPECT().UpdateState(gomock.Any(), state).Return(nil)
		server := etaskgrpc.NewReporterServer(executions)

		_, err := server.Report(context.Background(), &reporterv1.ReportRequest{
			ExecutionState: &executorv1.ExecutionState{
				Id: 10, TaskId: 20, Status: executorv1.ExecutionStatus_SUCCESS,
			},
		})
		require.NoError(t, err)
	})
}

func TestReporterDoesNotApplyStateWhenLogsFail(t *testing.T) {
	ctrl := gomock.NewController(t)
	executions := grpcmocks.NewMockExecutionReportHandler(ctrl)
	executions.EXPECT().AppendExecutionLogs(gomock.Any(), int64(10), int64(0), []string{"日志"}).
		Return(errors.New("数据库不可用"))
	server := etaskgrpc.NewReporterServer(executions)

	_, err := server.Report(context.Background(), &reporterv1.ReportRequest{
		ExecutionState: &executorv1.ExecutionState{Id: 10, Status: executorv1.ExecutionStatus_SUCCESS},
		LogChunks:      []string{"日志"},
	})
	require.Equal(t, codes.Internal, status.Code(err))
}
