// Package termination 管理工作流取消意图及物理终止信号的可靠投递。
package termination

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/Duke1616/eiam/pkg/gormx"
	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/repository"
	"github.com/Duke1616/etask/internal/service/invoker"
)

var (
	ErrInvalidCommand = errors.New("终止请求参数非法")
	ErrRejected       = errors.New("终止请求被拒绝")
)

//go:generate go tool mockgen -source=./service.go -package=terminationmocks -destination=./mocks/service.mock.go -typed

type Request struct {
	ExecutionID int64
	RequestID   string
	Reason      string
}

type Service interface {
	// Request 接受并持久化一次幂等终止请求。
	Request(ctx context.Context, request Request) error
	// Attach 在 execution 创建后应用可能早到的取消意图。
	Attach(ctx context.Context, execution domain.TaskExecution) (domain.TaskExecution, error)
	// DeliverPending 投递一批待发送的物理终止信号。
	DeliverPending(ctx context.Context, limit int) error
}

type service struct {
	cancellations repository.ExecutionCancellationRepository
	executions    repository.TaskExecutionRepository
	invoker       invoker.Invoker
}

func NewService(cancellations repository.ExecutionCancellationRepository,
	executions repository.TaskExecutionRepository, executionInvoker invoker.Invoker) Service {
	return &service{cancellations: cancellations, executions: executions, invoker: executionInvoker}
}

func (s *service) Request(ctx context.Context, request Request) error {
	request.RequestID = strings.TrimSpace(request.RequestID)
	request.Reason = strings.TrimSpace(request.Reason)
	if (request.ExecutionID <= 0 && request.RequestID == "") || request.Reason == "" ||
		len([]rune(request.Reason)) > 500 {
		return fmt.Errorf("%w: execution 标识非法或终止原因为空/超过 500 字", ErrInvalidCommand)
	}
	if ctxutil.GetTenantID(ctx).Int64() <= 0 {
		return fmt.Errorf("%w: 缺少租户上下文", ErrInvalidCommand)
	}
	err := s.cancellations.Request(ctx, request.ExecutionID,
		request.RequestID, request.Reason)
	if err != nil {
		if errors.Is(err, repository.ErrCancellationRejected) {
			return fmt.Errorf("%w: %v", ErrRejected, err)
		}
		return err
	}
	return nil
}

func (s *service) Attach(ctx context.Context,
	execution domain.TaskExecution) (domain.TaskExecution, error) {
	if !execution.Source.IsWorkflow() || execution.RequestID == "" {
		return execution, nil
	}
	return s.cancellations.Attach(ctx, execution)
}

func (s *service) DeliverPending(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 100
	}
	// 补偿器需要跨租户扫描；显式提权，避免行为依赖“空租户即全局”的插件约定。
	cancellations, err := s.cancellations.ListPending(gormx.IgnoreTenantContext(ctx), limit)
	if err != nil {
		return fmt.Errorf("查询待投递终止信号失败: %w", err)
	}
	var deliveryErrors []error
	for _, cancellation := range cancellations {
		if err := s.deliver(ctx, cancellation); err != nil {
			deliveryErrors = append(deliveryErrors, err)
		}
	}
	return errors.Join(deliveryErrors...)
}

func (s *service) deliver(ctx context.Context, cancellation domain.ExecutionCancellation) error {
	deliveryCtx := ctxutil.WithTenantID(ctx, cancellation.TenantID)
	deliveryCtx = ctxutil.WithOriginTenantID(deliveryCtx, cancellation.TenantID)
	execution, err := s.executions.GetByID(deliveryCtx, cancellation.ExecutionID)
	if err == nil {
		err = s.invoker.Terminate(deliveryCtx, execution, cancellation.Reason)
	}
	if err == nil {
		return s.cancellations.MarkSent(deliveryCtx, cancellation.ID)
	}
	nextAttemptAt := time.Now().Add(cancellationBackoff(cancellation.AttemptCount)).UnixMilli()
	if markErr := s.cancellations.MarkFailed(deliveryCtx, cancellation.ID,
		err.Error(), nextAttemptAt); markErr != nil {
		err = errors.Join(err, markErr)
	}
	return fmt.Errorf("execution %d 终止信号投递失败: %w", cancellation.ExecutionID, err)
}

func cancellationBackoff(attemptCount int) time.Duration {
	if attemptCount < 0 {
		attemptCount = 0
	}
	if attemptCount > 6 {
		attemptCount = 6
	}
	return time.Second * time.Duration(1<<attemptCount)
}
