package termination_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/Duke1616/etask/internal/domain"
	repositorymocks "github.com/Duke1616/etask/internal/repository/mocks"
	invokermocks "github.com/Duke1616/etask/internal/service/invoker/mocks"
	"github.com/Duke1616/etask/internal/service/termination"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestRequestPersistsIntentBeforeExecutionExists(t *testing.T) {
	ctrl := gomock.NewController(t)
	cancellations := repositorymocks.NewMockExecutionCancellationRepository(ctrl)
	executions := repositorymocks.NewMockTaskExecutionRepository(ctrl)
	physical := invokermocks.NewMockInvoker(ctrl)
	ctx := ctxutil.WithTenantID(context.Background(), 7)
	cancellations.EXPECT().Request(gomock.Any(), int64(0), "eflow:1:1", "管理员终止").Return(nil)
	service := termination.NewService(cancellations, executions, physical)

	err := service.Request(ctx, termination.Request{RequestID: "eflow:1:1", Reason: "管理员终止"})

	require.NoError(t, err)
}

func TestRequestExecutionPersistsCancellation(t *testing.T) {
	ctrl := gomock.NewController(t)
	cancellations := repositorymocks.NewMockExecutionCancellationRepository(ctrl)
	executions := repositorymocks.NewMockTaskExecutionRepository(ctrl)
	physical := invokermocks.NewMockInvoker(ctrl)
	ctx := ctxutil.WithTenantID(context.Background(), 7)
	cancellations.EXPECT().Request(gomock.Any(), int64(9), "", "参数配置错误").Return(nil)
	service := termination.NewService(cancellations, executions, physical)

	err := service.RequestExecution(ctx, 9, " 参数配置错误 ")

	require.NoError(t, err)
}

func TestRequestExecutionRejectsBlankReason(t *testing.T) {
	ctrl := gomock.NewController(t)
	service := termination.NewService(
		repositorymocks.NewMockExecutionCancellationRepository(ctrl),
		repositorymocks.NewMockTaskExecutionRepository(ctrl),
		invokermocks.NewMockInvoker(ctrl),
	)

	err := service.RequestExecution(ctxutil.WithTenantID(context.Background(), 7), 9, " ")

	require.ErrorIs(t, err, termination.ErrInvalidCommand)
}

func TestDeliverPendingMarksSuccessfulSignalSent(t *testing.T) {
	ctrl := gomock.NewController(t)
	cancellations := repositorymocks.NewMockExecutionCancellationRepository(ctrl)
	executions := repositorymocks.NewMockTaskExecutionRepository(ctrl)
	physical := invokermocks.NewMockInvoker(ctrl)
	pending := []domain.ExecutionCancellation{{
		ID: 1, TenantID: 7, ExecutionID: 9, Reason: "管理员终止",
	}}
	execution := domain.TaskExecution{ID: 9}
	gomock.InOrder(
		cancellations.EXPECT().ListPending(gomock.Any(), 10).Return(pending, nil),
		executions.EXPECT().GetByID(gomock.Any(), int64(9)).Return(execution, nil),
		physical.EXPECT().Terminate(gomock.Any(), execution, "管理员终止").
			DoAndReturn(func(ctx context.Context, _ domain.TaskExecution, _ string) error {
				require.Equal(t, int64(7), ctxutil.GetTenantID(ctx).Int64())
				return nil
			}),
		cancellations.EXPECT().MarkSent(gomock.Any(), int64(1)).Return(nil),
	)
	service := termination.NewService(cancellations, executions, physical)

	err := service.DeliverPending(context.Background(), 10)

	require.NoError(t, err)
}

func TestDeliverPendingSchedulesRetryAfterSignalFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	cancellations := repositorymocks.NewMockExecutionCancellationRepository(ctrl)
	executions := repositorymocks.NewMockTaskExecutionRepository(ctrl)
	physical := invokermocks.NewMockInvoker(ctrl)
	pending := []domain.ExecutionCancellation{{
		ID: 1, TenantID: 7, ExecutionID: 9, Reason: "管理员终止",
	}}
	execution := domain.TaskExecution{ID: 9}
	deliveryErr := errors.New("executor unavailable")
	gomock.InOrder(
		cancellations.EXPECT().ListPending(gomock.Any(), 10).Return(pending, nil),
		executions.EXPECT().GetByID(gomock.Any(), int64(9)).Return(execution, nil),
		physical.EXPECT().Terminate(gomock.Any(), execution, "管理员终止").Return(deliveryErr),
		cancellations.EXPECT().MarkFailed(gomock.Any(), int64(1), deliveryErr.Error(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ int64, _ string, nextAttemptAt int64) error {
				require.NotZero(t, nextAttemptAt)
				return nil
			}),
	)
	service := termination.NewService(cancellations, executions, physical)

	err := service.DeliverPending(context.Background(), 10)

	require.ErrorContains(t, err, "executor unavailable")
}
