package pool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/Duke1616/eiam/pkg/gormx"
	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/errs"
	"github.com/Duke1616/etask/internal/repository"
)

// BindingRequest 描述一次资源池授权绑定操作。
type BindingRequest struct {
	TenantID    int64
	PoolName    string
	HandlerName string
	Desc        string
}

// BindingManyRequest 描述一次批量资源池授权绑定操作。
type BindingManyRequest struct {
	TenantID     int64
	PoolName     string
	HandlerNames []string
	Desc         string
}

// BindingKey 描述一个资源池授权绑定的唯一业务键。
type BindingKey struct {
	TenantID    int64
	PoolName    string
	HandlerName string
}

// ListBindingsRequest 描述资源池授权绑定的查询条件。
type ListBindingsRequest struct {
	TenantID int64
	PoolName string
	Status   domain.ExecutionPoolBindingStatus
}

// CheckBindingRequest 描述一次运行时资源池授权检查。
type CheckBindingRequest struct {
	TenantID    int64
	PoolName    string
	HandlerName string
}

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
	// AdminList 查询管理视角下的资源池绑定；TenantID 为空时返回全量绑定。
	AdminList(ctx context.Context, req ListBindingsRequest) ([]domain.ExecutionPoolBinding, error)
	// IsAllowed 判断租户是否允许使用指定资源池和 handler。
	IsAllowed(ctx context.Context, req CheckBindingRequest) (bool, error)
	// RuntimeOverridableParameterKeys 返回 Handler 元数据中显式允许启动覆盖的参数键。
	RuntimeOverridableParameterKeys(ctx context.Context, req CheckBindingRequest) (map[string]struct{}, error)
}

type bindingService struct {
	poolRepo    repository.ExecutionPoolRepository
	bindingRepo repository.ExecutionPoolBindingRepository
}

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

func (s *bindingService) Bind(ctx context.Context, req BindingRequest) error {
	return s.BindMany(ctx, BindingManyRequest{
		TenantID:     req.TenantID,
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
	ctx = withTenant(ctx, req.TenantID)

	pool, err := s.findPool(ctx, poolName)
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
	bindings := make([]domain.ExecutionPoolBinding, 0, len(handlerNames))
	for _, name := range handlerNames {
		bindings = append(bindings, domain.ExecutionPoolBinding{
			PoolName:    poolName,
			HandlerName: name,
			Status:      domain.ExecutionPoolBindingStatusEnabled,
			Desc:        desc,
		})
	}
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

func (s *bindingService) Unbind(ctx context.Context, req BindingKey) error {
	poolName, handlerName, err := normalizeBindingKey(req.PoolName, req.HandlerName)
	if err != nil {
		return err
	}
	ctx = withTenant(ctx, req.TenantID)
	return s.bindingRepo.Unbind(ctx, poolName, handlerName)
}

func (s *bindingService) Enable(ctx context.Context, req BindingKey) error {
	return s.setStatus(ctx, req, domain.ExecutionPoolBindingStatusEnabled)
}

func (s *bindingService) Disable(ctx context.Context, req BindingKey) error {
	return s.setStatus(ctx, req, domain.ExecutionPoolBindingStatusDisabled)
}

func (s *bindingService) List(ctx context.Context, req ListBindingsRequest) ([]domain.ExecutionPoolBinding, error) {
	poolName := strings.TrimSpace(req.PoolName)
	ctx = withTenant(ctx, req.TenantID)
	if poolName == "" {
		return s.bindingRepo.List(ctx, req.Status)
	}
	return s.bindingRepo.ListByPool(ctx, poolName, req.Status)
}

func (s *bindingService) AdminList(ctx context.Context, req ListBindingsRequest) ([]domain.ExecutionPoolBinding, error) {
	poolName := strings.TrimSpace(req.PoolName)
	if req.TenantID > 0 {
		ctx = withTenant(ctx, req.TenantID)
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

func (s *bindingService) IsAllowed(ctx context.Context, req CheckBindingRequest) (bool, error) {
	poolName, handlerName, err := normalizeBindingKey(req.PoolName, req.HandlerName)
	if err != nil {
		return false, err
	}
	ctx = withTenant(ctx, req.TenantID)

	pool, err := s.findPool(ctx, poolName)
	if err != nil {
		if errors.Is(err, repository.ErrExecutionPoolNotFound) {
			return false, nil
		}
		return false, err
	}
	if pool.Status != domain.ExecutionPoolStatusEnabled {
		return false, nil
	}

	binding, err := s.bindingRepo.FindEffective(ctx, poolName, handlerName)
	if err != nil {
		if errors.Is(err, repository.ErrExecutionPoolBindingNotFound) {
			return false, nil
		}
		return false, err
	}
	return binding.Status == domain.ExecutionPoolBindingStatusEnabled, nil
}

func (s *bindingService) RuntimeOverridableParameterKeys(
	ctx context.Context,
	req CheckBindingRequest,
) (map[string]struct{}, error) {
	poolName, handlerName, err := normalizeBindingKey(req.PoolName, req.HandlerName)
	if err != nil {
		return nil, err
	}
	if handlerName == "" {
		return nil, fmt.Errorf("%w: handler 不能为空", errs.ErrInvalidParameter)
	}
	ctx = withTenant(ctx, req.TenantID)
	pool, err := s.findPool(ctx, poolName)
	if err != nil {
		return nil, err
	}

	var handlers []struct {
		Name     string `json:"name"`
		Metadata []struct {
			Key                string `json:"key"`
			RuntimeOverridable bool   `json:"runtime_overridable"`
		} `json:"metadata"`
	}
	if err = json.Unmarshal([]byte(pool.Metadata["supported_handlers"]), &handlers); err != nil {
		return nil, fmt.Errorf("解析资源池 %s Handler 元数据失败: %w", poolName, err)
	}
	for _, handler := range handlers {
		if strings.TrimSpace(handler.Name) != handlerName {
			continue
		}
		keys := make(map[string]struct{})
		for _, parameter := range handler.Metadata {
			key := strings.TrimSpace(parameter.Key)
			if parameter.RuntimeOverridable && key != "" {
				keys[key] = struct{}{}
			}
		}
		return keys, nil
	}
	return nil, fmt.Errorf("%w: handler %s 不属于资源池 %s", errs.ErrInvalidParameter, handlerName, poolName)
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
	ctx = withTenant(ctx, req.TenantID)
	return s.bindingRepo.SetStatus(ctx, poolName, handlerName, status)
}

func (s *bindingService) findPool(ctx context.Context, poolName string) (domain.ExecutionPool, error) {
	pool, err := s.poolRepo.Find(ctx, poolName)
	if err != nil {
		if errors.Is(err, repository.ErrExecutionPoolNotFound) {
			return domain.ExecutionPool{}, fmt.Errorf("%w: execution pool %s 不存在", err, poolName)
		}
		return domain.ExecutionPool{}, err
	}
	return pool, nil
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

	names := make([]string, 0, len(handlerNames))
	seen := make(map[string]struct{}, len(handlerNames))
	for _, name := range handlerNames {
		normalized := domain.NormalizeExecutionPoolHandlerName(name)
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		names = append(names, normalized)
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("%w: handler 不能为空", errs.ErrInvalidParameter)
	}
	if len(names) > 1 {
		for _, name := range names {
			if name == "" {
				return nil, fmt.Errorf("%w: * 不能和具体 handler 同时授权", errs.ErrInvalidParameter)
			}
		}
	}
	return names, nil
}

func validatePoolHandlers(pool domain.ExecutionPool, handlerNames []string) error {
	supported, ok := supportedHandlerSet(pool)
	if !ok {
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

func supportedHandlerSet(pool domain.ExecutionPool) (map[string]struct{}, bool) {
	if pool.Metadata == nil {
		return nil, false
	}
	raw := strings.TrimSpace(pool.Metadata["supported_handlers"])
	if raw == "" {
		return nil, false
	}

	var handlers []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(raw), &handlers); err != nil || len(handlers) == 0 {
		return nil, false
	}

	res := make(map[string]struct{}, len(handlers))
	for _, handler := range handlers {
		name := domain.NormalizeExecutionPoolHandlerName(handler.Name)
		if name != "" {
			res[name] = struct{}{}
		}
	}
	return res, len(res) > 0
}

func (s *bindingService) ensureBindingCreatable(ctx context.Context, poolName string, handlerNames []string) error {
	existing, err := s.bindingRepo.ListByPool(ctx, poolName, "")
	if err != nil {
		return err
	}
	if len(existing) == 0 {
		return nil
	}

	existingHandlers := make(map[string]struct{}, len(existing))
	for _, binding := range existing {
		existingHandlers[domain.NormalizeExecutionPoolHandlerName(binding.HandlerName)] = struct{}{}
	}
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

func withTenant(ctx context.Context, tenantID int64) context.Context {
	if tenantID <= 0 {
		return ctx
	}
	return ctxutil.WithTenantID(ctx, tenantID)
}
