package pool

import (
	"context"
	"errors"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/repository"
)

// poolAuthorizer 只负责执行资源池使用权判断。
type poolAuthorizer struct {
	poolRepo    repository.ExecutionPoolRepository
	bindingRepo repository.ExecutionPoolBindingRepository
}

// NewExecutionPoolAuthorizer 创建执行资源池授权检查器。
func NewExecutionPoolAuthorizer(poolRepo repository.ExecutionPoolRepository,
	bindingRepo repository.ExecutionPoolBindingRepository) ExecutionPoolAuthorizer {
	return &poolAuthorizer{poolRepo: poolRepo, bindingRepo: bindingRepo}
}

// IsAllowed 判断租户是否允许使用指定资源池和 Handler。
func (a *poolAuthorizer) IsAllowed(ctx context.Context, req CheckBindingRequest) (bool, error) {
	poolName, handlerName, err := normalizeBindingKey(req.PoolName, req.HandlerName)
	if err != nil {
		return false, err
	}
	p, err := a.poolRepo.Find(ctx, poolName)
	if err != nil {
		if errors.Is(err, repository.ErrExecutionPoolNotFound) {
			return false, nil
		}
		return false, err
	}
	if p.Status != domain.ExecutionPoolStatusEnabled {
		return false, nil
	}

	binding, err := a.bindingRepo.FindEffective(ctx, poolName, handlerName)
	if err != nil {
		if errors.Is(err, repository.ErrExecutionPoolBindingNotFound) {
			return false, nil
		}
		return false, err
	}
	return binding.Status == domain.ExecutionPoolBindingStatusEnabled, nil
}

var _ ExecutionPoolAuthorizer = (*poolAuthorizer)(nil)
