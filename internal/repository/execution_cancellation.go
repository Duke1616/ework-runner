package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/repository/dao"
)

// ErrCancellationRejected 表示目标 execution 不接受当前取消请求。
var ErrCancellationRejected = errors.New("execution 取消请求被拒绝")

//go:generate go tool mockgen -source=./execution_cancellation.go -package=repositorymocks -destination=./mocks/execution_cancellation.mock.go -typed

// ExecutionCancellationRepository 保存取消意图，并与 execution 状态原子绑定。
type ExecutionCancellationRepository interface {
	// Request 持久化取消意图；execution 尚未创建时等待后续 Attach。
	Request(ctx context.Context, executionID int64, requestID, reason string) error
	// Attach 将早到的取消意图绑定到新建 execution，并返回数据库中的最终状态。
	Attach(ctx context.Context, execution domain.TaskExecution) (domain.TaskExecution, error)
	// ListPending 跨租户查询一批到期的待投递终止信号。
	ListPending(ctx context.Context, limit int) ([]domain.ExecutionCancellation, error)
	// MarkSent 将当前租户下的终止信号标记为已发送。
	MarkSent(ctx context.Context, id int64) error
	// MarkFailed 保存当前租户下的投递错误和下次重试时间。
	MarkFailed(ctx context.Context, id int64, reason string, nextAttemptAt int64) error
}

type executionCancellationRepository struct {
	dao        dao.ExecutionCancellationDAO
	executions TaskExecutionRepository
}

func NewExecutionCancellationRepository(cancellationDAO dao.ExecutionCancellationDAO,
	executions TaskExecutionRepository) ExecutionCancellationRepository {
	return &executionCancellationRepository{dao: cancellationDAO, executions: executions}
}

func (r *executionCancellationRepository) Request(ctx context.Context, executionID int64,
	requestID, reason string) error {
	return translateCancellationError(r.dao.Request(ctx, executionID, requestID, reason))
}

func (r *executionCancellationRepository) Attach(ctx context.Context,
	execution domain.TaskExecution) (domain.TaskExecution, error) {
	attached, err := r.dao.Attach(ctx, execution.ID, execution.RequestID)
	err = translateCancellationError(err)
	if err != nil || !attached {
		return execution, err
	}
	return r.executions.GetByID(ctx, execution.ID)
}

func translateCancellationError(err error) error {
	if errors.Is(err, dao.ErrCancellationTerminal) {
		return fmt.Errorf("%w: %w", ErrCancellationRejected, err)
	}
	return err
}

func (r *executionCancellationRepository) ListPending(ctx context.Context,
	limit int) ([]domain.ExecutionCancellation, error) {
	entities, err := r.dao.ListPending(ctx, limit)
	result := make([]domain.ExecutionCancellation, 0, len(entities))
	for _, entity := range entities {
		result = append(result, toCancellationDomain(entity))
	}
	return result, err
}

func (r *executionCancellationRepository) MarkSent(ctx context.Context, id int64) error {
	return r.dao.MarkSent(ctx, id)
}

func (r *executionCancellationRepository) MarkFailed(ctx context.Context, id int64, reason string,
	nextAttemptAt int64) error {
	return r.dao.MarkFailed(ctx, id, reason, nextAttemptAt)
}

func toCancellationDomain(entity dao.ExecutionCancellation) domain.ExecutionCancellation {
	return domain.ExecutionCancellation{
		ID: entity.ID, TenantID: entity.TenantID, RequestID: entity.RequestID,
		ExecutionID: entity.ExecutionID.Int64, Reason: entity.Reason,
		DeliveryStatus: domain.CancellationDeliveryStatus(entity.DeliveryStatus),
		AttemptCount:   entity.AttemptCount, NextAttemptAt: entity.NextAttemptAt,
		LastError: entity.LastError, CTime: entity.CTime, UTime: entity.UTime,
	}
}
