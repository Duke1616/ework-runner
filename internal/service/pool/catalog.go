package pool

import (
	"context"
	"strings"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/repository"
	"github.com/samber/lo"
	clientv3 "go.etcd.io/etcd/client/v3"
)

const defaultCatalogLimit int64 = 10

// CatalogListRequest 描述当前租户可见资源池分页查询条件。
type CatalogListRequest struct {
	Kind    domain.ExecutionPoolKind
	Offset  int64
	Limit   int64
	Keyword string
}

// PoolListRequest 描述管理端资源池分页查询条件。
type PoolListRequest struct {
	Offset       int64
	Limit        int64
	Keyword      string
	Kind         domain.ExecutionPoolKind
	Transport    domain.ExecutionTransport
	DispatchMode domain.ExecMode
	Status       domain.ExecutionPoolStatus
}

// CatalogPage 描述当前租户可见资源池分页结果。
type CatalogPage struct {
	Pools    []domain.ExecutionPool
	Bindings []domain.ExecutionPoolBinding
	Total    int64
}

// PoolPage 描述管理端资源池分页结果。
type PoolPage struct {
	Pools []domain.ExecutionPool
	Total int64
}

// Node 描述注册中心里当前在线的资源实例。
type Node struct {
	ID      string
	Address string
}

// CatalogService 提供当前租户可见执行资源目录。
type CatalogService interface {
	// ListAuthorizedPools 分页查询当前租户有权使用的可用资源池。
	ListAuthorizedPools(ctx context.Context, req CatalogListRequest) (CatalogPage, error)
	// ListPools 分页查询资源池，供管理端使用。
	ListPools(ctx context.Context, req PoolListRequest) (PoolPage, error)
	// ListNodes 查询资源池当前在线节点。
	ListNodes(ctx context.Context, pool domain.ExecutionPool) ([]Node, error)
	// ListNodesForPools 批量查询资源池当前在线节点。
	ListNodesForPools(ctx context.Context, pools []domain.ExecutionPool) (map[string][]Node, error)
}

type catalogService struct {
	poolRepo    repository.ExecutionPoolRepository
	bindingRepo repository.ExecutionPoolBindingRepository
	etcd        *clientv3.Client
}

// NewCatalogService 创建执行资源目录服务。
func NewCatalogService(
	poolRepo repository.ExecutionPoolRepository,
	bindingRepo repository.ExecutionPoolBindingRepository,
	etcd *clientv3.Client,
) CatalogService {
	return &catalogService{
		poolRepo:    poolRepo,
		bindingRepo: bindingRepo,
		etcd:        etcd,
	}
}

func (s *catalogService) ListAuthorizedPools(ctx context.Context, req CatalogListRequest) (CatalogPage, error) {
	limit := normalizeCatalogLimit(req.Limit)
	keyword := strings.TrimSpace(req.Keyword)
	return s.listAuthorizedPoolsByOffset(ctx, req, limit, keyword)
}

func (s *catalogService) listAuthorizedPoolsByOffset(
	ctx context.Context,
	req CatalogListRequest,
	limit int64,
	keyword string,
) (CatalogPage, error) {
	offset := req.Offset
	if offset < 0 {
		offset = 0
	}
	names, err := s.bindingRepo.ListAuthorizedPoolNames(
		ctx,
		offset,
		limit,
		keyword,
		req.Kind,
		domain.ExecutionPoolBindingStatusEnabled,
		domain.ExecutionPoolStatusEnabled,
	)
	if err != nil {
		return CatalogPage{}, err
	}
	total, err := s.bindingRepo.CountAuthorizedPoolNames(
		ctx,
		keyword,
		req.Kind,
		domain.ExecutionPoolBindingStatusEnabled,
		domain.ExecutionPoolStatusEnabled,
	)
	if err != nil {
		return CatalogPage{}, err
	}
	pools, err := s.poolRepo.ListByNames(ctx, names, req.Kind, domain.ExecutionPoolStatusEnabled, keyword)
	if err != nil {
		return CatalogPage{}, err
	}
	byName := lo.SliceToMap(pools, func(pool domain.ExecutionPool) (string, domain.ExecutionPool) {
		return pool.Name, pool
	})
	pools = lo.FilterMap(names, func(name string, _ int) (domain.ExecutionPool, bool) {
		pool, ok := byName[name]
		return pool, ok
	})

	bindings, err := s.bindingRepo.ListByPoolNames(
		ctx,
		lo.Map(pools, func(pool domain.ExecutionPool, _ int) string { return pool.Name }),
		"",
	)
	if err != nil {
		return CatalogPage{}, err
	}
	return CatalogPage{
		Pools:    pools,
		Bindings: bindings,
		Total:    total,
	}, nil
}

func (s *catalogService) ListPools(ctx context.Context, req PoolListRequest) (PoolPage, error) {
	limit := normalizeCatalogLimit(req.Limit)
	pools, err := s.poolRepo.List(ctx, req.Offset, limit, strings.TrimSpace(req.Keyword),
		req.Kind, req.Transport, req.DispatchMode, req.Status)
	if err != nil {
		return PoolPage{}, err
	}
	total, err := s.poolRepo.Count(ctx, strings.TrimSpace(req.Keyword), req.Kind,
		req.Transport, req.DispatchMode, req.Status)
	if err != nil {
		return PoolPage{}, err
	}
	return PoolPage{
		Pools: pools,
		Total: total,
	}, nil
}

func normalizeCatalogLimit(limit int64) int64 {
	if limit <= 0 {
		return defaultCatalogLimit
	}
	return limit
}
