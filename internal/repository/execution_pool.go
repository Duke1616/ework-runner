package repository

import (
	"context"
	"errors"

	"github.com/Duke1616/eiam/pkg/gormx"
	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/repository/dao"
	"github.com/Duke1616/etask/pkg/sqlx"
	"github.com/samber/lo"
	"gorm.io/gorm"
)

// ErrExecutionPoolNotFound 表示执行资源池不存在。
var ErrExecutionPoolNotFound = gorm.ErrRecordNotFound

// ErrExecutionPoolBindingNotFound 表示执行资源池绑定不存在。
var ErrExecutionPoolBindingNotFound = gorm.ErrRecordNotFound

//go:generate go tool mockgen -source=./execution_pool.go -package=repositorymocks -destination=./mocks/execution_pool.mock.go -exclude_interfaces=ExecutionPoolBindingRepository -typed

// ExecutionPoolRepository 封装执行资源池的持久化访问。
type ExecutionPoolRepository interface {
	// Upsert 按资源池名称创建或更新资源池。
	Upsert(ctx context.Context, pool domain.ExecutionPool) error
	// Disable 按资源池名称禁用资源池。
	Disable(ctx context.Context, name string) error
	// Find 按资源池名称查询资源池。
	Find(ctx context.Context, name string) (domain.ExecutionPool, error)
	// ListByKind 查询指定资源类型下的资源池。
	ListByKind(ctx context.Context, kind domain.ExecutionPoolKind) ([]domain.ExecutionPool, error)
	// ListByNames 按名称列表查询资源池。
	ListByNames(
		ctx context.Context,
		names []string,
		kind domain.ExecutionPoolKind,
		status domain.ExecutionPoolStatus,
		keyword string,
	) ([]domain.ExecutionPool, error)
	// List 分页查询资源池。
	List(
		ctx context.Context,
		offset int64,
		limit int64,
		keyword string,
		kind domain.ExecutionPoolKind,
		transport domain.ExecutionTransport,
		dispatchMode domain.ExecMode,
		status domain.ExecutionPoolStatus,
	) ([]domain.ExecutionPool, error)
	// Count 统计资源池数量。
	Count(
		ctx context.Context,
		keyword string,
		kind domain.ExecutionPoolKind,
		transport domain.ExecutionTransport,
		dispatchMode domain.ExecMode,
		status domain.ExecutionPoolStatus,
	) (int64, error)
}

type executionPoolRepository struct {
	poolDAO dao.ExecutionPoolDAO
}

// ExecutionPoolBindingRepository 封装租户对执行资源池的授权绑定访问。
type ExecutionPoolBindingRepository interface {
	// Bind 创建或更新当前上下文租户的资源池绑定。
	Bind(ctx context.Context, binding domain.ExecutionPoolBinding) error
	// Create 创建当前上下文租户的资源池绑定；已存在时由唯一索引返回冲突错误。
	Create(ctx context.Context, binding domain.ExecutionPoolBinding) error
	// CreateBatch 原子创建当前上下文租户的一组资源池绑定。
	CreateBatch(ctx context.Context, bindings []domain.ExecutionPoolBinding) error
	// CreateDedicatedBatch 原子检查并创建专属资源池绑定；occupied 为 true 表示已被其他租户占用。
	CreateDedicatedBatch(ctx context.Context, tenantID int64, poolName string, bindings []domain.ExecutionPoolBinding) (occupied bool, err error)
	// Unbind 删除当前上下文租户下指定资源池绑定。
	Unbind(ctx context.Context, poolName, handlerName string) error
	// SetStatus 更新当前上下文租户下指定资源池绑定的启停状态。
	SetStatus(ctx context.Context, poolName, handlerName string, status domain.ExecutionPoolBindingStatus) error
	// FindEffective 查询运行时生效的绑定：精确 handler 优先，其次整池绑定。
	FindEffective(ctx context.Context, poolName, handlerName string) (domain.ExecutionPoolBinding, error)
	// List 查询当前上下文租户的资源池绑定列表。
	List(ctx context.Context, status domain.ExecutionPoolBindingStatus) ([]domain.ExecutionPoolBinding, error)
	// ListByPool 查询当前上下文租户下指定资源池的绑定列表。
	ListByPool(ctx context.Context, poolName string, status domain.ExecutionPoolBindingStatus) ([]domain.ExecutionPoolBinding, error)
	// AdminList 查询管理视角下的资源池绑定列表，调用方负责提供忽略租户隔离的 context。
	AdminList(ctx context.Context, status domain.ExecutionPoolBindingStatus) ([]domain.ExecutionPoolBinding, error)
	// AdminListByPool 查询管理视角下指定资源池的绑定列表，调用方负责提供忽略租户隔离的 context。
	AdminListByPool(ctx context.Context, poolName string, status domain.ExecutionPoolBindingStatus) ([]domain.ExecutionPoolBinding, error)
	// ListByPoolNames 查询当前上下文租户下多个资源池的绑定列表。
	ListByPoolNames(ctx context.Context, poolNames []string, status domain.ExecutionPoolBindingStatus) ([]domain.ExecutionPoolBinding, error)
	// ListPoolNames 查询当前上下文租户已绑定的资源池名称列表。
	ListPoolNames(ctx context.Context, cursor string, limit int64, status domain.ExecutionPoolBindingStatus) ([]string, error)
	// ListAuthorizedPoolNames 分页查询当前上下文租户有权使用的资源池名称。
	ListAuthorizedPoolNames(
		ctx context.Context,
		offset int64,
		limit int64,
		keyword string,
		kind domain.ExecutionPoolKind,
		bindingStatus domain.ExecutionPoolBindingStatus,
		poolStatus domain.ExecutionPoolStatus,
	) ([]string, error)
	// CountAuthorizedPoolNames 统计当前上下文租户有权使用的资源池数量。
	CountAuthorizedPoolNames(
		ctx context.Context,
		keyword string,
		kind domain.ExecutionPoolKind,
		bindingStatus domain.ExecutionPoolBindingStatus,
		poolStatus domain.ExecutionPoolStatus,
	) (int64, error)
}

type executionPoolBindingRepository struct {
	bindingDAO dao.ExecutionPoolBindingDAO
}

// NewExecutionPoolRepository 创建执行资源池仓储。
func NewExecutionPoolRepository(poolDAO dao.ExecutionPoolDAO) ExecutionPoolRepository {
	return &executionPoolRepository{poolDAO: poolDAO}
}

// NewExecutionPoolBindingRepository 创建执行资源池绑定仓储。
func NewExecutionPoolBindingRepository(bindingDAO dao.ExecutionPoolBindingDAO) ExecutionPoolBindingRepository {
	return &executionPoolBindingRepository{bindingDAO: bindingDAO}
}

func (r *executionPoolRepository) Upsert(ctx context.Context, pool domain.ExecutionPool) error {
	return r.poolDAO.UpsertByName(ctx, r.toDAO(pool))
}

func (r *executionPoolRepository) Disable(ctx context.Context, name string) error {
	_, err := r.poolDAO.SetStatus(ctx, name, domain.ExecutionPoolStatusDisabled.String())
	return err
}

func (r *executionPoolRepository) Find(ctx context.Context, name string) (domain.ExecutionPool, error) {
	pool, err := r.poolDAO.FindByName(ctx, name)
	if err == nil {
		return r.toDomain(pool), nil
	}
	if !errors.Is(err, ErrExecutionPoolNotFound) {
		return domain.ExecutionPool{}, err
	}
	// 兼容历史 KAFKA Runner：旧 target 保存的是 Topic，新模型统一保存资源池名称。
	pool, err = r.poolDAO.FindMQByTopic(ctx, name)
	if err != nil {
		return domain.ExecutionPool{}, err
	}
	return r.toDomain(pool), nil
}

func (r *executionPoolRepository) ListByKind(ctx context.Context, kind domain.ExecutionPoolKind) ([]domain.ExecutionPool, error) {
	pools, err := r.poolDAO.ListByKind(ctx, kind.String())
	if err != nil {
		return nil, err
	}

	return lo.Map(pools, func(pool dao.ExecutionPool, _ int) domain.ExecutionPool {
		return r.toDomain(pool)
	}), nil
}

func (r *executionPoolRepository) ListByNames(
	ctx context.Context,
	names []string,
	kind domain.ExecutionPoolKind,
	status domain.ExecutionPoolStatus,
	keyword string,
) ([]domain.ExecutionPool, error) {
	pools, err := r.poolDAO.ListByNames(ctx, names, keyword, kind.String(), "", "", status.String())
	if err != nil {
		return nil, err
	}
	return lo.Map(pools, func(pool dao.ExecutionPool, _ int) domain.ExecutionPool {
		return r.toDomain(pool)
	}), nil
}

func (r *executionPoolRepository) List(
	ctx context.Context,
	offset int64,
	limit int64,
	keyword string,
	kind domain.ExecutionPoolKind,
	transport domain.ExecutionTransport,
	dispatchMode domain.ExecMode,
	status domain.ExecutionPoolStatus,
) ([]domain.ExecutionPool, error) {
	pools, err := r.poolDAO.List(ctx, offset, limit, keyword, kind.String(),
		transport.String(), dispatchMode.String(), status.String())
	if err != nil {
		return nil, err
	}
	return lo.Map(pools, func(pool dao.ExecutionPool, _ int) domain.ExecutionPool {
		return r.toDomain(pool)
	}), nil
}

func (r *executionPoolRepository) Count(
	ctx context.Context,
	keyword string,
	kind domain.ExecutionPoolKind,
	transport domain.ExecutionTransport,
	dispatchMode domain.ExecMode,
	status domain.ExecutionPoolStatus,
) (int64, error) {
	return r.poolDAO.Count(ctx, keyword, kind.String(), transport.String(), dispatchMode.String(), status.String())
}

func (r *executionPoolRepository) toDAO(pool domain.ExecutionPool) dao.ExecutionPool {
	return dao.ExecutionPool{
		ID:             pool.ID,
		Name:           pool.Name,
		Kind:           pool.Kind.String(),
		Transport:      pool.Transport.String(),
		DispatchMode:   pool.DispatchMode.String(),
		IsolationLevel: pool.IsolationLevel.String(),
		Desc:           pool.Desc,
		Status:         pool.Status.String(),
		Metadata: sqlx.JSONColumn[map[string]string]{
			Val:   pool.Metadata,
			Valid: pool.Metadata != nil,
		},
		CTime: pool.CTime,
		UTime: pool.UTime,
	}
}

func (r *executionPoolRepository) toDomain(pool dao.ExecutionPool) domain.ExecutionPool {
	return domain.ExecutionPool{
		ID:             pool.ID,
		Name:           pool.Name,
		Kind:           domain.ExecutionPoolKind(pool.Kind),
		Transport:      domain.ExecutionTransport(pool.Transport),
		DispatchMode:   domain.ExecMode(pool.DispatchMode),
		IsolationLevel: domain.ExecutionPoolIsolation(pool.IsolationLevel),
		Desc:           pool.Desc,
		Status:         domain.ExecutionPoolStatus(pool.Status),
		Metadata:       pool.Metadata.Val,
		CTime:          pool.CTime,
		UTime:          pool.UTime,
	}
}

func (r *executionPoolBindingRepository) Bind(ctx context.Context, binding domain.ExecutionPoolBinding) error {
	binding.HandlerName = domain.NormalizeExecutionPoolHandlerName(binding.HandlerName)
	return r.bindingDAO.Upsert(ctx, r.toDAO(binding))
}

func (r *executionPoolBindingRepository) Create(ctx context.Context, binding domain.ExecutionPoolBinding) error {
	binding.HandlerName = domain.NormalizeExecutionPoolHandlerName(binding.HandlerName)
	_, err := r.bindingDAO.Create(ctx, r.toDAO(binding))
	return err
}

func (r *executionPoolBindingRepository) CreateBatch(ctx context.Context, bindings []domain.ExecutionPoolBinding) error {
	entities := make([]dao.ExecutionPoolBinding, 0, len(bindings))
	for _, binding := range bindings {
		binding.HandlerName = domain.NormalizeExecutionPoolHandlerName(binding.HandlerName)
		entities = append(entities, r.toDAO(binding))
	}
	return r.bindingDAO.CreateBatch(ctx, entities)
}

func (r *executionPoolBindingRepository) CreateDedicatedBatch(
	ctx context.Context,
	tenantID int64,
	poolName string,
	bindings []domain.ExecutionPoolBinding,
) (bool, error) {
	entities := make([]dao.ExecutionPoolBinding, 0, len(bindings))
	for _, binding := range bindings {
		binding.HandlerName = domain.NormalizeExecutionPoolHandlerName(binding.HandlerName)
		entities = append(entities, r.toDAO(binding))
	}
	return r.bindingDAO.CreateDedicatedBatch(gormx.IgnoreTenantContext(ctx), tenantID, poolName, entities)
}

func (r *executionPoolBindingRepository) Unbind(ctx context.Context, poolName, handlerName string) error {
	affected, err := r.bindingDAO.Delete(ctx, poolName, domain.NormalizeExecutionPoolHandlerName(handlerName))
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrExecutionPoolBindingNotFound
	}
	return nil
}

func (r *executionPoolBindingRepository) SetStatus(
	ctx context.Context,
	poolName string,
	handlerName string,
	status domain.ExecutionPoolBindingStatus,
) error {
	affected, err := r.bindingDAO.SetStatus(ctx, poolName, domain.NormalizeExecutionPoolHandlerName(handlerName), status.String())
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrExecutionPoolBindingNotFound
	}
	return nil
}

func (r *executionPoolBindingRepository) FindEffective(
	ctx context.Context,
	poolName string,
	handlerName string,
) (domain.ExecutionPoolBinding, error) {
	binding, err := r.bindingDAO.FindEffective(ctx, poolName, domain.NormalizeExecutionPoolHandlerName(handlerName))
	if err != nil {
		return domain.ExecutionPoolBinding{}, err
	}
	return r.toBindingDomain(binding), nil
}

func (r *executionPoolBindingRepository) List(
	ctx context.Context,
	status domain.ExecutionPoolBindingStatus,
) ([]domain.ExecutionPoolBinding, error) {
	bindings, err := r.bindingDAO.List(ctx, status.String())
	if err != nil {
		return nil, err
	}
	return lo.Map(bindings, func(binding dao.ExecutionPoolBinding, _ int) domain.ExecutionPoolBinding {
		return r.toBindingDomain(binding)
	}), nil
}

func (r *executionPoolBindingRepository) ListByPool(
	ctx context.Context,
	poolName string,
	status domain.ExecutionPoolBindingStatus,
) ([]domain.ExecutionPoolBinding, error) {
	bindings, err := r.bindingDAO.ListByPool(ctx, poolName, status.String())
	if err != nil {
		return nil, err
	}
	return lo.Map(bindings, func(binding dao.ExecutionPoolBinding, _ int) domain.ExecutionPoolBinding {
		return r.toBindingDomain(binding)
	}), nil
}

func (r *executionPoolBindingRepository) AdminList(
	ctx context.Context,
	status domain.ExecutionPoolBindingStatus,
) ([]domain.ExecutionPoolBinding, error) {
	return r.List(ctx, status)
}

func (r *executionPoolBindingRepository) AdminListByPool(
	ctx context.Context,
	poolName string,
	status domain.ExecutionPoolBindingStatus,
) ([]domain.ExecutionPoolBinding, error) {
	return r.ListByPool(ctx, poolName, status)
}

func (r *executionPoolBindingRepository) ListByPoolNames(
	ctx context.Context,
	poolNames []string,
	status domain.ExecutionPoolBindingStatus,
) ([]domain.ExecutionPoolBinding, error) {
	bindings, err := r.bindingDAO.ListByPoolNames(ctx, poolNames, status.String())
	if err != nil {
		return nil, err
	}
	return lo.Map(bindings, func(binding dao.ExecutionPoolBinding, _ int) domain.ExecutionPoolBinding {
		return r.toBindingDomain(binding)
	}), nil
}

func (r *executionPoolBindingRepository) ListPoolNames(
	ctx context.Context,
	cursor string,
	limit int64,
	status domain.ExecutionPoolBindingStatus,
) ([]string, error) {
	return r.bindingDAO.ListPoolNames(ctx, cursor, limit, status.String())
}

func (r *executionPoolBindingRepository) ListAuthorizedPoolNames(
	ctx context.Context,
	offset int64,
	limit int64,
	keyword string,
	kind domain.ExecutionPoolKind,
	bindingStatus domain.ExecutionPoolBindingStatus,
	poolStatus domain.ExecutionPoolStatus,
) ([]string, error) {
	return r.bindingDAO.ListAuthorizedPoolNames(
		ctx,
		offset,
		limit,
		keyword,
		kind.String(),
		bindingStatus.String(),
		poolStatus.String(),
	)
}

func (r *executionPoolBindingRepository) CountAuthorizedPoolNames(
	ctx context.Context,
	keyword string,
	kind domain.ExecutionPoolKind,
	bindingStatus domain.ExecutionPoolBindingStatus,
	poolStatus domain.ExecutionPoolStatus,
) (int64, error) {
	return r.bindingDAO.CountAuthorizedPoolNames(
		ctx,
		keyword,
		kind.String(),
		bindingStatus.String(),
		poolStatus.String(),
	)
}

func (r *executionPoolBindingRepository) toDAO(binding domain.ExecutionPoolBinding) dao.ExecutionPoolBinding {
	return dao.ExecutionPoolBinding{
		ID:          binding.ID,
		TenantID:    binding.TenantID,
		PoolName:    binding.PoolName,
		HandlerName: domain.NormalizeExecutionPoolHandlerName(binding.HandlerName),
		Status:      binding.Status.String(),
		Desc:        binding.Desc,
		CTime:       binding.CTime,
		UTime:       binding.UTime,
	}
}

func (r *executionPoolBindingRepository) toBindingDomain(binding dao.ExecutionPoolBinding) domain.ExecutionPoolBinding {
	return domain.ExecutionPoolBinding{
		ID:          binding.ID,
		TenantID:    binding.TenantID,
		PoolName:    binding.PoolName,
		HandlerName: binding.HandlerName,
		Status:      domain.ExecutionPoolBindingStatus(binding.Status),
		Desc:        binding.Desc,
		CTime:       binding.CTime,
		UTime:       binding.UTime,
	}
}
