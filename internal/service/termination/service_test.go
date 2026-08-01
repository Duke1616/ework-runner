package termination

import (
	"context"
	"errors"
	"testing"

	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/repository"
	"github.com/Duke1616/etask/internal/service/invoker"
	"github.com/stretchr/testify/require"
)

func TestRequestPersistsIntentBeforeExecutionExists(t *testing.T) {
	cancellations := &cancellationRepositoryStub{}
	service := NewService(cancellations, &executionRepositoryStub{}, &invokerStub{})
	ctx := ctxutil.WithTenantID(context.Background(), 7)

	err := service.Request(ctx, Request{RequestID: "eflow:1:1", Reason: "管理员终止"})

	require.NoError(t, err)
	require.Equal(t, "eflow:1:1", cancellations.requestID)
}

func TestDeliverPendingMarksSuccessfulSignalSent(t *testing.T) {
	cancellations := &cancellationRepositoryStub{pending: []domain.ExecutionCancellation{{
		ID: 1, TenantID: 7, ExecutionID: 9, Reason: "管理员终止",
	}}}
	executions := &executionRepositoryStub{execution: domain.TaskExecution{ID: 9}}
	physical := &invokerStub{}
	service := NewService(cancellations, executions, physical)

	err := service.DeliverPending(context.Background(), 10)

	require.NoError(t, err)
	require.Equal(t, int64(1), cancellations.sentID)
	require.Equal(t, 1, physical.terminateCalls)
	require.Equal(t, int64(7), physical.tenantID)
}

func TestDeliverPendingSchedulesRetryAfterSignalFailure(t *testing.T) {
	cancellations := &cancellationRepositoryStub{pending: []domain.ExecutionCancellation{{
		ID: 1, TenantID: 7, ExecutionID: 9, Reason: "管理员终止",
	}}}
	physical := &invokerStub{err: errors.New("executor unavailable")}
	service := NewService(cancellations,
		&executionRepositoryStub{execution: domain.TaskExecution{ID: 9}}, physical)

	err := service.DeliverPending(context.Background(), 10)

	require.ErrorContains(t, err, "executor unavailable")
	require.Equal(t, int64(1), cancellations.failedID)
	require.NotZero(t, cancellations.nextAttemptAt)
}

type cancellationRepositoryStub struct {
	repository.ExecutionCancellationRepository
	pending       []domain.ExecutionCancellation
	requestID     string
	sentID        int64
	failedID      int64
	nextAttemptAt int64
}

func (s *cancellationRepositoryStub) Request(_ context.Context, _ int64,
	requestID, _ string) error {
	s.requestID = requestID
	return nil
}

func (s *cancellationRepositoryStub) ListPending(context.Context,
	int) ([]domain.ExecutionCancellation, error) {
	return s.pending, nil
}

func (s *cancellationRepositoryStub) MarkSent(_ context.Context, id int64) error {
	s.sentID = id
	return nil
}

func (s *cancellationRepositoryStub) MarkFailed(_ context.Context, id int64, _ string,
	nextAttemptAt int64) error {
	s.failedID = id
	s.nextAttemptAt = nextAttemptAt
	return nil
}

type executionRepositoryStub struct {
	repository.TaskExecutionRepository
	execution domain.TaskExecution
}

func (s *executionRepositoryStub) GetByID(context.Context, int64) (domain.TaskExecution, error) {
	return s.execution, nil
}

type invokerStub struct {
	invoker.Invoker
	err            error
	terminateCalls int
	tenantID       int64
}

func (s *invokerStub) Terminate(ctx context.Context, _ domain.TaskExecution, _ string) error {
	s.terminateCalls++
	s.tenantID = ctxutil.GetTenantID(ctx).Int64()
	return s.err
}
