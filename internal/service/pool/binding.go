package pool

import (
	"context"
	"fmt"
	"strings"

	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/Duke1616/eiam/pkg/gormx"
	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/errs"
	"github.com/Duke1616/etask/internal/repository"
	"github.com/samber/lo"
)

// BindingRequest 描述一次资源池授权绑定操作，目标租户从 Context 获取。
type BindingRequest struct {
	PoolName    string
	HandlerName string
	Desc        string
}

// BindingManyRequest 描述一次批量资源池授权绑定操作，目标租户从 Context 获取。
type BindingManyRequest struct {
	PoolName     string
	HandlerNames []string
	Desc         string
}

// BindingKey 描述一个资源池授权绑定的业务键，租户从 Context 获取。
type BindingKey struct {
	PoolName    string
	HandlerName string
}

// ListBindingsRequest 描述资源池授权绑定的查询条件。
type ListBindingsRequest struct {
	PoolName   string
	Status     domain.ExecutionPoolBindingStatus
	AllTenants bool // 管理查询是否忽略租户隔离。
}

// CheckBindingRequest 描述一次运行时资源池授权检查，租户从 Context 获取。
type CheckBindingRequest = HandlerQuery

// BindingService 维护租户和执行资源池之间的授权关系。
type BindingService interface {
	// Bind 为租户创建资源池绑定；handler 为空或 * 时表示授权整个资源池。
	Bind(ctx context.Context, req BindingRequest) error
	// BindMany 为租户创建一组资源池绑定；handler 为空或 * 时表示授权整个资源池。
	BindMany(ctx context.Context, req BindingManyRequest) error
	// Unbind 删除租户的资源池绑定。
	Unbind(ctx context.Context, req BindingKey) error
	// Enable 启用租户的资源池绑定。
	Enable(ctx context.Context, req BindingKey) error
	// Disable 禁用租户的资源池绑定。
	Disable(ctx context.Context, req BindingKey) error
	// List 查询租户的资源池绑定。
	List(ctx context.Context, req ListBindingsRequest) ([]domain.ExecutionPoolBinding, error)
	// AdminList 查询管理视角下的资源池绑定；AllTenants 为 true 时返回全量绑定。
	AdminList(ctx context.Context, req ListBindingsRequest) ([]domain.ExecutionPoolBinding, error)
}

type bindingService struct {
	poolRepo    repository.ExecutionPoolRepository
	bindingRepo repository.ExecutionPoolBindingRepository
}

var _ BindingService = (*bindingService)(nil)

// NewBindingService 创建执行资源池绑定服务。
func NewBindingService(
	poolRepo repository.ExecutionPoolRepository,
	bindingRepo repository.ExecutionPoolBindingRepository,
) BindingService {
	return &bindingService{
		poolRepo:    poolRepo,
		bindingRepo: bindingRepo,
	}
}

// -------------------- 绑定生命周期 --------------------

func (s *bindingService) Bind(ctx context.Context, req BindingRequest) error {
	return s.BindMany(ctx, BindingManyRequest{
		PoolName:     req.PoolName,
		HandlerNames: []string{req.HandlerName},
		Desc:         req.Desc,
	})
}

func (s *bindingService) BindMany(ctx context.Context, req BindingManyRequest) error {
	poolName := strings.TrimSpace(req.PoolName)
	if poolName == "" {
		return fmt.Errorf("%w: execution pool 不能为空", errs.ErrInvalidParameter)
	}
	handlerNames, err := normalizeBindingHandlers(req.HandlerNames)
	if err != nil {
		return err
	}
	pool, err := s.poolRepo.Find(ctx, poolName)
	if err != nil {
		return err
	}
	if err = validatePoolHandlers(pool, handlerNames); err != nil {
		return err
	}
	if err = s.ensureBindingCreatable(ctx, poolName, handlerNames); err != nil {
		return err
	}
	desc := strings.TrimSpace(req.Desc)
	bindings := lo.Map(handlerNames, func(name string, _ int) domain.ExecutionPoolBinding {
		return domain.ExecutionPoolBinding{
			PoolName:    poolName,
			HandlerName: name,
			Status:      domain.ExecutionPoolBindingStatusEnabled,
			Desc:        desc,
		}
	})
	if pool.IsolationLevel == domain.ExecutionPoolIsolationDedicated {
		tenantID := ctxutil.GetTenantID(ctx).Int64()
		occupied, err := s.bindingRepo.CreateDedicatedBatch(ctx, tenantID, poolName, bindings)
		if err != nil {
			return err
		}
		if occupied {
			return fmt.Errorf("%w: 专属资源池 %s 已被其他租户占用", errs.ErrInvalidParameter, poolName)
		}
		return nil
	}
	return s.bindingRepo.CreateBatch(ctx, bindings)
}

// -------------------- 绑定状态操作 --------------------

func (s *bindingService) Unbind(ctx context.Context, req BindingKey) error {
	poolName, handlerName, err := normalizeBindingKey(req.PoolName, req.HandlerName)
	if err != nil {
		return err
	}
	return s.bindingRepo.Unbind(ctx, poolName, handlerName)
}

func (s *bindingService) Enable(ctx context.Context, req BindingKey) error {
	return s.setStatus(ctx, req, domain.ExecutionPoolBindingStatusEnabled)
}

func (s *bindingService) Disable(ctx context.Context, req BindingKey) error {
	return s.setStatus(ctx, req, domain.ExecutionPoolBindingStatusDisabled)
}

// -------------------- 绑定查询 --------------------

func (s *bindingService) List(ctx context.Context, req ListBindingsRequest) ([]domain.ExecutionPoolBinding, error) {
	poolName := strings.TrimSpace(req.PoolName)
	if poolName == "" {
		return s.bindingRepo.List(ctx, req.Status)
	}
	return s.bindingRepo.ListByPool(ctx, poolName, req.Status)
}

func (s *bindingService) AdminList(ctx context.Context, req ListBindingsRequest) ([]domain.ExecutionPoolBinding, error) {
	poolName := strings.TrimSpace(req.PoolName)
	if !req.AllTenants {
		if poolName == "" {
			return s.bindingRepo.List(ctx, req.Status)
		}
		return s.bindingRepo.ListByPool(ctx, poolName, req.Status)
	}

	ctx = gormx.IgnoreTenantContext(ctx)
	if poolName == "" {
		return s.bindingRepo.AdminList(ctx, req.Status)
	}
	return s.bindingRepo.AdminListByPool(ctx, poolName, req.Status)
}

func (s *bindingService) setStatus(
	ctx context.Context,
	req BindingKey,
	status domain.ExecutionPoolBindingStatus,
) error {
	poolName, handlerName, err := normalizeBindingKey(req.PoolName, req.HandlerName)
	if err != nil {
		return err
	}
	return s.bindingRepo.SetStatus(ctx, poolName, handlerName, status)
}

func normalizeBindingKey(poolName, handlerName string) (string, string, error) {
	poolName = strings.TrimSpace(poolName)
	if poolName == "" {
		return "", "", fmt.Errorf("%w: execution pool 不能为空", errs.ErrInvalidParameter)
	}
	return poolName, domain.NormalizeExecutionPoolHandlerName(handlerName), nil
}

func normalizeBindingHandlers(handlerNames []string) ([]string, error) {
	if len(handlerNames) == 0 {
		return nil, fmt.Errorf("%w: handler 不能为空", errs.ErrInvalidParameter)
	}

	names := lo.Uniq(lo.Map(handlerNames, func(name string, _ int) string {
		return domain.NormalizeExecutionPoolHandlerName(name)
	}))
	if len(names) > 1 {
		if lo.Contains(names, "") {
			return nil, fmt.Errorf("%w: * 不能和具体 handler 同时授权", errs.ErrInvalidParameter)
		}
	}
	return names, nil
}

func validatePoolHandlers(pool domain.ExecutionPool, handlerNames []string) error {
	supported, declared, err := supportedHandlerSet(pool)
	if err != nil {
		return err
	}
	if !declared {
		return nil
	}
	for _, name := range handlerNames {
		if name == "" {
			continue
		}
		if _, exists := supported[name]; !exists {
			return fmt.Errorf("%w: handler %s 不属于资源池 %s", errs.ErrInvalidParameter, name, pool.Name)
		}
	}
	return nil
}

func supportedHandlerSet(pool domain.ExecutionPool) (map[string]struct{}, bool, error) {
	handlers, ok, err := parseHandlerMetadata(pool)
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, false, nil
	}

	names := lo.FilterMap(handlers, func(handler HandlerMetadata, _ int) (string, bool) {
		name := domain.NormalizeExecutionPoolHandlerName(handler.Name)
		return name, name != ""
	})
	res := lo.SliceToMap(names, func(name string) (string, struct{}) {
		return name, struct{}{}
	})
	return res, len(res) > 0, nil
}

func (s *bindingService) ensureBindingCreatable(ctx context.Context, poolName string, handlerNames []string) error {
	existing, err := s.bindingRepo.ListByPool(ctx, poolName, "")
	if err != nil {
		return err
	}
	if len(existing) == 0 {
		return nil
	}

	existingHandlers := lo.SliceToMap(existing, func(binding domain.ExecutionPoolBinding) (string, struct{}) {
		return domain.NormalizeExecutionPoolHandlerName(binding.HandlerName), struct{}{}
	})
	if _, ok := existingHandlers[""]; ok {
		return fmt.Errorf("%w: 资源池 %s 已授权全部 handler", errs.ErrInvalidParameter, poolName)
	}

	for _, name := range handlerNames {
		if name == "" {
			return fmt.Errorf("%w: 资源池 %s 已存在 handler 授权，不能再授权全部 handler", errs.ErrInvalidParameter, poolName)
		}
		if _, ok := existingHandlers[name]; ok {
			return fmt.Errorf("%w: handler %s 已授权", errs.ErrInvalidParameter, name)
		}
	}
	return nil
}
