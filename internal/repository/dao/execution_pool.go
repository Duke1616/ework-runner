package dao

import (
	"context"
	"errors"
	"time"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/pkg/sqlx"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ExecutionPool 表示可被租户授权使用的一组执行资源。
// Name 对应 gRPC executor service name 或 Agent 消费侧的逻辑通道名。
type ExecutionPool struct {
	ID             int64                              `gorm:"column:id;type:bigint;primaryKey;autoIncrement;comment:'执行资源池自增ID'"`
	Name           string                             `gorm:"column:name;type:varchar(128);not null;uniqueIndex:uniq_execution_pools_name;comment:'执行资源池名称，对应 executor service name 或 agent 通道名'"`
	Kind           string                             `gorm:"column:kind;type:ENUM('EXECUTOR','AGENT');not null;comment:'执行资源类型: EXECUTOR/AGENT'"`
	Transport      string                             `gorm:"column:transport;type:ENUM('GRPC','MQ');not null;comment:'任务传输通道: GRPC/MQ'"`
	DispatchMode   string                             `gorm:"column:dispatch_mode;type:ENUM('PUSH','PULL');not null;default:'PUSH';comment:'任务派发方式: PUSH/PULL'"`
	IsolationLevel string                             `gorm:"column:isolation_level;type:ENUM('SHARED','DEDICATED');not null;default:'SHARED';comment:'隔离级别: SHARED/DEDICATED'"`
	Desc           string                             `gorm:"column:desc;type:text;comment:'执行资源池描述'"`
	Status         string                             `gorm:"column:status;type:ENUM('ENABLED','DISABLED');not null;default:'ENABLED';index;comment:'资源池状态'"`
	Metadata       sqlx.JSONColumn[map[string]string] `gorm:"column:metadata;type:json;comment:'扩展元数据'"`
	CTime          int64                              `gorm:"column:ctime;type:bigint;comment:'创建时间(毫秒)'"`
	UTime          int64                              `gorm:"column:utime;type:bigint;comment:'更新时间(毫秒)'"`
}

func (ExecutionPool) TableName() string {
	return "execution_pools"
}

// ExecutionPoolBinding 表示当前租户被授权使用某个执行资源池。
// HandlerName 为空表示允许使用该资源池下的全部 handler。
type ExecutionPoolBinding struct {
	ID          int64  `gorm:"column:id;type:bigint;primaryKey;autoIncrement;comment:'租户执行资源绑定自增ID'"`
	TenantID    int64  `gorm:"column:tenant_id;type:bigint unsigned;not null;index;uniqueIndex:uniq_tenant_pool_handler,priority:1;comment:'租户ID'"`
	PoolName    string `gorm:"column:pool_name;type:varchar(128);not null;index;uniqueIndex:uniq_tenant_pool_handler,priority:2;comment:'执行资源池名称'"`
	HandlerName string `gorm:"column:handler_name;type:varchar(128);not null;default:'';uniqueIndex:uniq_tenant_pool_handler,priority:3;comment:'允许的 handler，空表示全部'"`
	Status      string `gorm:"column:status;type:ENUM('ENABLED','DISABLED');not null;default:'ENABLED';index;comment:'绑定状态'"`
	Desc        string `gorm:"column:desc;type:text;comment:'绑定说明'"`
	CTime       int64  `gorm:"column:ctime;type:bigint;comment:'创建时间(毫秒)'"`
	UTime       int64  `gorm:"column:utime;type:bigint;comment:'更新时间(毫秒)'"`
}

func (ExecutionPoolBinding) TableName() string {
	return "execution_pool_bindings"
}

type ExecutionPoolDAO interface {
	// Create 创建执行资源池并返回自增 ID。
	Create(ctx context.Context, pool ExecutionPool) (int64, error)
	// UpsertByName 按资源池名称创建或更新资源池。
	UpsertByName(ctx context.Context, pool ExecutionPool) error
	// Update 按资源池名称更新资源池配置。
	Update(ctx context.Context, pool ExecutionPool) (int64, error)
	// SetStatus 按资源池名称更新启停状态。
	SetStatus(ctx context.Context, name string, status string) (int64, error)
	// FindByName 按资源池名称查询资源池。
	FindByName(ctx context.Context, name string) (ExecutionPool, error)
	// FindMQByTopic 按兼容的 MQ Topic 查询资源池。
	FindMQByTopic(ctx context.Context, topic string) (ExecutionPool, error)
	// List 按筛选条件分页查询资源池列表。
	List(ctx context.Context, offset, limit int64, keyword, kind, transport, dispatchMode, status string) ([]ExecutionPool, error)
	// ListByKind 按执行资源类型查询全部资源池。
	ListByKind(ctx context.Context, kind string) ([]ExecutionPool, error)
	// ListByNames 按名称列表查询资源池。
	ListByNames(ctx context.Context, names []string, keyword, kind, transport, dispatchMode, status string) ([]ExecutionPool, error)
	// Count 按筛选条件统计资源池数量。
	Count(ctx context.Context, keyword, kind, transport, dispatchMode, status string) (int64, error)
	// DeleteByName 按资源池名称删除资源池。
	DeleteByName(ctx context.Context, name string) (int64, error)
}

type ExecutionPoolBindingDAO interface {
	// Create 为当前上下文租户创建执行资源池绑定，tenant_id 由 GORM 租户插件自动填充。
	Create(ctx context.Context, binding ExecutionPoolBinding) (int64, error)
	// CreateBatch 原子创建当前上下文租户的一组资源池绑定。
	CreateBatch(ctx context.Context, bindings []ExecutionPoolBinding) error
	// CreateDedicatedBatch 原子检查并创建专属资源池绑定；occupied 为 true 表示已被其他租户占用。
	CreateDedicatedBatch(ctx context.Context, tenantID int64, poolName string, bindings []ExecutionPoolBinding) (occupied bool, err error)
	// Upsert 为当前上下文租户创建或更新执行资源池绑定。
	Upsert(ctx context.Context, binding ExecutionPoolBinding) error
	// SetStatus 更新当前上下文租户下指定资源池绑定的启停状态。
	SetStatus(ctx context.Context, poolName, handlerName, status string) (int64, error)
	// Delete 删除当前上下文租户下指定资源池绑定。
	Delete(ctx context.Context, poolName, handlerName string) (int64, error)
	// List 查询当前上下文租户的资源池绑定列表。
	List(ctx context.Context, status string) ([]ExecutionPoolBinding, error)
	// ListByPool 查询当前上下文租户下指定资源池的绑定列表；跨租户管理场景可配合 gormx.IgnoreTenantContext 使用。
	ListByPool(ctx context.Context, poolName string, status string) ([]ExecutionPoolBinding, error)
	// ListByPoolNames 查询当前上下文租户下多个资源池的绑定列表。
	ListByPoolNames(ctx context.Context, poolNames []string, status string) ([]ExecutionPoolBinding, error)
	// ListPoolNames 查询当前上下文租户已绑定的资源池名称列表。
	ListPoolNames(ctx context.Context, cursor string, limit int64, status string) ([]string, error)
	// ListAuthorizedPoolNames 分页查询当前上下文租户有权使用的资源池名称。
	ListAuthorizedPoolNames(ctx context.Context, offset, limit int64, keyword, kind, bindingStatus, poolStatus string) ([]string, error)
	// CountAuthorizedPoolNames 统计当前上下文租户有权使用的资源池数量。
	CountAuthorizedPoolNames(ctx context.Context, keyword, kind, bindingStatus, poolStatus string) (int64, error)
	// FindEffective 查询运行时生效的绑定：精确 handler 优先，其次为空 handler 的整池绑定。
	FindEffective(ctx context.Context, poolName, handlerName string) (ExecutionPoolBinding, error)
}

type GORMExecutionPoolDAO struct {
	db *gorm.DB
}

func NewGORMExecutionPoolDAO(db *gorm.DB) ExecutionPoolDAO {
	return &GORMExecutionPoolDAO{db: db}
}

func (g *GORMExecutionPoolDAO) Create(ctx context.Context, pool ExecutionPool) (int64, error) {
	now := time.Now().UnixMilli()
	pool.CTime, pool.UTime = now, now
	if pool.Status == "" {
		pool.Status = domain.ExecutionPoolStatusEnabled.String()
	}
	err := g.db.WithContext(ctx).Create(&pool).Error
	return pool.ID, err
}

func (g *GORMExecutionPoolDAO) UpsertByName(ctx context.Context, pool ExecutionPool) error {
	var existing ExecutionPool
	err := g.db.WithContext(ctx).Where("name = ?", pool.Name).First(&existing).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			_, err = g.Create(ctx, pool)
		}
		return err
	}
	pool.ID = existing.ID
	_, err = g.Update(ctx, pool)
	return err
}

func (g *GORMExecutionPoolDAO) Update(ctx context.Context, pool ExecutionPool) (int64, error) {
	updates := map[string]any{
		"kind":            pool.Kind,
		"transport":       pool.Transport,
		"dispatch_mode":   pool.DispatchMode,
		"isolation_level": pool.IsolationLevel,
		"desc":            pool.Desc,
		"metadata":        pool.Metadata,
		"utime":           time.Now().UnixMilli(),
	}
	if pool.Status != "" {
		updates["status"] = pool.Status
	}

	res := g.db.WithContext(ctx).
		Model(&ExecutionPool{}).
		Where("name = ?", pool.Name).
		Updates(updates)
	return res.RowsAffected, res.Error
}

func (g *GORMExecutionPoolDAO) SetStatus(ctx context.Context, name string, status string) (int64, error) {
	res := g.db.WithContext(ctx).
		Model(&ExecutionPool{}).
		Where("name = ?", name).
		Updates(map[string]any{
			"status": status,
			"utime":  time.Now().UnixMilli(),
		})
	return res.RowsAffected, res.Error
}

func (g *GORMExecutionPoolDAO) FindByName(ctx context.Context, name string) (ExecutionPool, error) {
	var pool ExecutionPool
	err := g.db.WithContext(ctx).Where("name = ?", name).First(&pool).Error
	return pool, err
}

func (g *GORMExecutionPoolDAO) FindMQByTopic(ctx context.Context, topic string) (ExecutionPool, error) {
	var pool ExecutionPool
	err := g.db.WithContext(ctx).
		Where("transport = ? AND JSON_UNQUOTE(JSON_EXTRACT(metadata, '$.topic')) = ?",
			domain.ExecutionTransportMQ.String(), topic).
		First(&pool).Error
	return pool, err
}

func (g *GORMExecutionPoolDAO) List(ctx context.Context, offset, limit int64,
	keyword, kind, transport, dispatchMode, status string) ([]ExecutionPool, error) {
	var pools []ExecutionPool
	err := g.buildPoolQuery(ctx, keyword, kind, transport, dispatchMode, status).
		Order("utime DESC").
		Offset(int(offset)).
		Limit(int(limit)).
		Find(&pools).Error
	return pools, err
}

func (g *GORMExecutionPoolDAO) ListByKind(ctx context.Context, kind string) ([]ExecutionPool, error) {
	var pools []ExecutionPool
	err := g.db.WithContext(ctx).
		Model(&ExecutionPool{}).
		Where("kind = ?", kind).
		Order("utime DESC").
		Find(&pools).Error
	return pools, err
}

func (g *GORMExecutionPoolDAO) ListByNames(
	ctx context.Context,
	names []string,
	keyword string,
	kind string,
	transport string,
	dispatchMode string,
	status string,
) ([]ExecutionPool, error) {
	if len(names) == 0 {
		return nil, nil
	}
	var pools []ExecutionPool
	err := g.buildPoolQuery(ctx, keyword, kind, transport, dispatchMode, status).
		Where("name IN ?", names).
		Find(&pools).Error
	return pools, err
}

func (g *GORMExecutionPoolDAO) Count(ctx context.Context,
	keyword, kind, transport, dispatchMode, status string) (int64, error) {
	var total int64
	err := g.buildPoolQuery(ctx, keyword, kind, transport, dispatchMode, status).
		Count(&total).Error
	return total, err
}

func (g *GORMExecutionPoolDAO) DeleteByName(ctx context.Context, name string) (int64, error) {
	res := g.db.WithContext(ctx).Where("name = ?", name).Delete(&ExecutionPool{})
	return res.RowsAffected, res.Error
}

func (g *GORMExecutionPoolDAO) buildPoolQuery(ctx context.Context,
	keyword, kind, transport, dispatchMode, status string) *gorm.DB {
	db := g.db.WithContext(ctx).Model(&ExecutionPool{})
	if keyword != "" {
		like := "%" + keyword + "%"
		db = db.Where("name LIKE ? OR `desc` LIKE ? OR CAST(metadata AS CHAR) LIKE ?", like, like, like)
	}
	if kind != "" {
		db = db.Where("kind = ?", kind)
	}
	if transport != "" {
		db = db.Where("transport = ?", transport)
	}
	if dispatchMode != "" {
		db = db.Where("dispatch_mode = ?", dispatchMode)
	}
	if status != "" {
		db = db.Where("status = ?", status)
	}
	return db
}

type GORMExecutionPoolBindingDAO struct {
	db *gorm.DB
}

func NewGORMExecutionPoolBindingDAO(db *gorm.DB) ExecutionPoolBindingDAO {
	return &GORMExecutionPoolBindingDAO{db: db}
}

func (g *GORMExecutionPoolBindingDAO) Create(ctx context.Context, binding ExecutionPoolBinding) (int64, error) {
	now := time.Now().UnixMilli()
	binding.CTime, binding.UTime = now, now
	if binding.Status == "" {
		binding.Status = domain.ExecutionPoolBindingStatusEnabled.String()
	}
	err := g.db.WithContext(ctx).Create(&binding).Error
	return binding.ID, err
}

func (g *GORMExecutionPoolBindingDAO) CreateBatch(ctx context.Context, bindings []ExecutionPoolBinding) error {
	if len(bindings) == 0 {
		return nil
	}
	now := time.Now().UnixMilli()
	for i := range bindings {
		bindings[i].CTime, bindings[i].UTime = now, now
		if bindings[i].Status == "" {
			bindings[i].Status = domain.ExecutionPoolBindingStatusEnabled.String()
		}
	}
	return g.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.Create(&bindings).Error
	})
}

func (g *GORMExecutionPoolBindingDAO) CreateDedicatedBatch(
	ctx context.Context,
	tenantID int64,
	poolName string,
	bindings []ExecutionPoolBinding,
) (bool, error) {
	if len(bindings) == 0 {
		return false, nil
	}
	now := time.Now().UnixMilli()
	for i := range bindings {
		bindings[i].TenantID = tenantID
		bindings[i].CTime, bindings[i].UTime = now, now
		if bindings[i].Status == "" {
			bindings[i].Status = domain.ExecutionPoolBindingStatusEnabled.String()
		}
	}

	var occupied bool
	err := g.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 对 pool 行加锁，使“检查其他租户 + 创建绑定”对同一资源池串行化。
		var pool ExecutionPool
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("name = ?", poolName).First(&pool).Error; err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&ExecutionPoolBinding{}).
			Where("pool_name = ? AND tenant_id <> ?", poolName, tenantID).
			Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			occupied = true
			return nil
		}
		return tx.Create(&bindings).Error
	})
	return occupied, err
}

func (g *GORMExecutionPoolBindingDAO) Upsert(ctx context.Context, binding ExecutionPoolBinding) error {
	now := time.Now().UnixMilli()
	binding.CTime, binding.UTime = now, now
	if binding.Status == "" {
		binding.Status = domain.ExecutionPoolBindingStatusEnabled.String()
	}

	return g.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "tenant_id"},
				{Name: "pool_name"},
				{Name: "handler_name"},
			},
			DoUpdates: clause.Assignments(map[string]any{
				"status": binding.Status,
				"desc":   binding.Desc,
				"utime":  now,
			}),
		}).
		Create(&binding).Error
}

func (g *GORMExecutionPoolBindingDAO) SetStatus(ctx context.Context, poolName, handlerName, status string) (int64, error) {
	res := g.db.WithContext(ctx).
		Model(&ExecutionPoolBinding{}).
		Where("pool_name = ? AND handler_name = ?", poolName, handlerName).
		Updates(map[string]any{
			"status": status,
			"utime":  time.Now().UnixMilli(),
		})
	return res.RowsAffected, res.Error
}

func (g *GORMExecutionPoolBindingDAO) Delete(ctx context.Context, poolName, handlerName string) (int64, error) {
	res := g.db.WithContext(ctx).
		Where("pool_name = ? AND handler_name = ?", poolName, handlerName).
		Delete(&ExecutionPoolBinding{})
	return res.RowsAffected, res.Error
}

func (g *GORMExecutionPoolBindingDAO) List(ctx context.Context, status string) ([]ExecutionPoolBinding, error) {
	var bindings []ExecutionPoolBinding
	db := g.db.WithContext(ctx)
	if status != "" {
		db = db.Where("status = ?", status)
	}
	err := db.Order("utime DESC").Find(&bindings).Error
	return bindings, err
}

func (g *GORMExecutionPoolBindingDAO) ListByPool(ctx context.Context, poolName string, status string) ([]ExecutionPoolBinding, error) {
	var bindings []ExecutionPoolBinding
	db := g.db.WithContext(ctx).
		Where("pool_name = ?", poolName)
	if status != "" {
		db = db.Where("status = ?", status)
	}
	err := db.Order("utime DESC").Find(&bindings).Error
	return bindings, err
}

func (g *GORMExecutionPoolBindingDAO) ListByPoolNames(
	ctx context.Context,
	poolNames []string,
	status string,
) ([]ExecutionPoolBinding, error) {
	if len(poolNames) == 0 {
		return nil, nil
	}
	var bindings []ExecutionPoolBinding
	db := g.db.WithContext(ctx).
		Where("pool_name IN ?", poolNames)
	if status != "" {
		db = db.Where("status = ?", status)
	}
	err := db.Order("pool_name ASC, handler_name ASC").Find(&bindings).Error
	return bindings, err
}

func (g *GORMExecutionPoolBindingDAO) ListPoolNames(
	ctx context.Context,
	cursor string,
	limit int64,
	status string,
) ([]string, error) {
	var names []string
	db := g.db.WithContext(ctx).
		Model(&ExecutionPoolBinding{}).
		Distinct("pool_name")
	if status != "" {
		db = db.Where("status = ?", status)
	}
	if cursor != "" {
		db = db.Where("pool_name > ?", cursor)
	}
	err := db.Order("pool_name ASC").
		Limit(int(limit)).
		Pluck("pool_name", &names).Error
	return names, err
}

func (g *GORMExecutionPoolBindingDAO) ListAuthorizedPoolNames(
	ctx context.Context,
	offset int64,
	limit int64,
	keyword string,
	kind string,
	bindingStatus string,
	poolStatus string,
) ([]string, error) {
	var names []string
	db := g.authorizedPoolNamesQuery(ctx, keyword, kind, bindingStatus, poolStatus)
	err := db.Order("execution_pool_bindings.pool_name ASC").
		Offset(int(offset)).
		Limit(int(limit)).
		Pluck("execution_pool_bindings.pool_name", &names).Error
	return names, err
}

func (g *GORMExecutionPoolBindingDAO) CountAuthorizedPoolNames(
	ctx context.Context,
	keyword string,
	kind string,
	bindingStatus string,
	poolStatus string,
) (int64, error) {
	var total int64
	err := g.authorizedPoolNamesQuery(ctx, keyword, kind, bindingStatus, poolStatus).Count(&total).Error
	return total, err
}

func (g *GORMExecutionPoolBindingDAO) authorizedPoolNamesQuery(
	ctx context.Context,
	keyword string,
	kind string,
	bindingStatus string,
	poolStatus string,
) *gorm.DB {
	db := g.db.WithContext(ctx).
		Model(&ExecutionPoolBinding{}).
		Distinct("execution_pool_bindings.pool_name").
		Joins("JOIN execution_pools ON execution_pools.name = execution_pool_bindings.pool_name")
	if bindingStatus != "" {
		db = db.Where("execution_pool_bindings.status = ?", bindingStatus)
	}
	if poolStatus != "" {
		db = db.Where("execution_pools.status = ?", poolStatus)
	}
	if kind != "" {
		db = db.Where("execution_pools.kind = ?", kind)
	}
	if keyword != "" {
		like := "%" + keyword + "%"
		db = db.Where("(execution_pools.name LIKE ? OR execution_pools.`desc` LIKE ?)", like, like)
	}
	return db
}

func (g *GORMExecutionPoolBindingDAO) Find(ctx context.Context, poolName, handlerName string) (ExecutionPoolBinding, error) {
	var binding ExecutionPoolBinding
	err := g.db.WithContext(ctx).
		Where("pool_name = ? AND handler_name = ?", poolName, handlerName).
		First(&binding).Error
	return binding, err
}

func (g *GORMExecutionPoolBindingDAO) FindEffective(
	ctx context.Context,
	poolName string,
	handlerName string,
) (ExecutionPoolBinding, error) {
	handlerName = domain.NormalizeExecutionPoolHandlerName(handlerName)
	if handlerName == "" {
		return g.Find(ctx, poolName, "")
	}

	var binding ExecutionPoolBinding
	err := g.db.WithContext(ctx).
		Where("pool_name = ? AND handler_name IN ?", poolName, []string{handlerName, ""}).
		Order(clause.Expr{
			SQL:                "CASE WHEN handler_name = ? THEN 0 ELSE 1 END",
			Vars:               []any{handlerName},
			WithoutParentheses: true,
		}).
		First(&binding).Error
	return binding, err
}
