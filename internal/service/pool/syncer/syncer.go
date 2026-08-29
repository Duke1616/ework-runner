package syncer

import (
	"context"
	"time"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/repository"
	poolsource "github.com/Duke1616/etask/internal/service/pool/syncer/source"
	"github.com/Duke1616/etask/pkg/grpc/registry"
	"github.com/gotomicro/ego/core/elog"
	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
)

const (
	syncTimeout        = 5 * time.Second
	disableGracePeriod = 2 * time.Second
	watchRetryDelay    = time.Second
	watchBufferSize    = 64
)

type eventKind uint8

const (
	eventUpsert eventKind = iota + 1
	eventDelete
)

type sourceEvent struct {
	source   poolsource.Source
	kind     eventKind
	instance registry.ServiceInstance
}

type sourceSnapshot struct {
	active       map[string]struct{}
	nextRevision int64
}

// Syncer 将注册中心里的 executor/agent 运行态资源同步为 execution_pools。
// 它只维护资源池可用性，不修改 execution_pool_bindings 里的租户授权关系。
type Syncer struct {
	poolRepo repository.ExecutionPoolRepository
	etcd     *clientv3.Client
	logger   *elog.Component
}

// NewSyncer 创建 ExecutionPool 同步任务。
func NewSyncer(poolRepo repository.ExecutionPoolRepository, etcd *clientv3.Client) *Syncer {
	return &Syncer{
		poolRepo: poolRepo,
		etcd:     etcd,
		logger:   elog.DefaultLogger.With(elog.FieldComponentName("pool.Syncer")),
	}
}

// Start 先全量对齐 etcd 状态，再持续监听注册实例的新增、更新和删除。
func (s *Syncer) Start(ctx context.Context) {
	sources := s.sources()
	revisions := s.syncAll(ctx, sources)

	events := make(chan sourceEvent, watchBufferSize)
	for _, source := range sources {
		go s.watchSource(ctx, source, revisions[source.Name()], events)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			s.handleEvent(ctx, event)
		}
	}
}

func (s *Syncer) sources() []poolsource.Source {
	return []poolsource.Source{
		poolsource.NewExecutor(s.etcd),
		poolsource.NewAgent(s.etcd),
	}
}

func (s *Syncer) syncAll(ctx context.Context, sources []poolsource.Source) map[string]int64 {
	revisions := make(map[string]int64, len(sources))
	for _, source := range sources {
		if revision, ok := s.resyncSource(ctx, source); ok {
			revisions[source.Name()] = revision
		}
	}
	return revisions
}

func (s *Syncer) resyncSource(ctx context.Context, source poolsource.Source) (int64, bool) {
	syncCtx, cancel := context.WithTimeout(ctx, syncTimeout)
	defer cancel()

	snapshot, err := s.syncSource(syncCtx, source)
	if err != nil {
		s.logger.Warn("同步 execution pools 失败",
			elog.String("source", source.Name()),
			elog.FieldErr(err))
		return 0, false
	}
	s.disableMissing(syncCtx, source, snapshot.active)
	return snapshot.nextRevision, true
}

func (s *Syncer) syncSource(ctx context.Context, source poolsource.Source) (sourceSnapshot, error) {
	resp, err := s.etcd.Get(ctx, source.Prefix(), clientv3.WithPrefix())
	if err != nil {
		return sourceSnapshot{}, err
	}

	instances := make(map[string][]domain.ExecutionPool)
	for _, kv := range resp.Kvs {
		inst, ok := poolsource.DecodeInstance(kv)
		if !ok || !source.Accept(inst) {
			continue
		}
		pool, ok := source.BuildPool(inst)
		if !ok {
			continue
		}
		instances[pool.Name] = append(instances[pool.Name], pool)
	}

	active := make(map[string]struct{}, len(instances))
	for name, pools := range instances {
		pool, aggregateErr := aggregatePoolInstances(pools)
		if aggregateErr != nil {
			s.logger.Error("执行资源池实例配置不一致，资源池将被禁用",
				elog.String("source", source.Name()), elog.String("pool", name), elog.FieldErr(aggregateErr))
			continue
		}
		active[name] = struct{}{}
		s.upsertPool(ctx, source, pool)
	}
	return sourceSnapshot{
		active:       active,
		nextRevision: resp.Header.Revision + 1,
	}, nil
}

func (s *Syncer) watchSource(ctx context.Context, source poolsource.Source, revision int64, events chan<- sourceEvent) {
	nextRevision := revision
	for {
		opts := []clientv3.OpOption{clientv3.WithPrefix(), clientv3.WithPrevKV()}
		if nextRevision > 0 {
			opts = append(opts, clientv3.WithRev(nextRevision))
		}

		watch := s.etcd.Watch(ctx, source.Prefix(), opts...)
		for resp := range watch {
			if resp.Err() != nil {
				s.logger.Warn("监听 registry prefix 失败",
					elog.String("source", source.Name()),
					elog.String("prefix", source.Prefix()),
					elog.FieldErr(resp.Err()))
				if revision, ok := s.resyncSource(ctx, source); ok {
					nextRevision = revision
				} else {
					nextRevision = 0
				}
				break
			}
			if resp.Header.Revision > 0 {
				nextRevision = resp.Header.Revision + 1
			}

			for _, etcdEvent := range resp.Events {
				event, ok := toSourceEvent(source, etcdEvent)
				if !ok {
					continue
				}
				select {
				case events <- event:
				case <-ctx.Done():
					return
				}
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(watchRetryDelay):
		}
	}
}

func (s *Syncer) handleEvent(ctx context.Context, event sourceEvent) {
	switch event.kind {
	case eventUpsert:
		s.resyncSource(ctx, event.source)
	case eventDelete:
		s.disableIfNoInstances(ctx, event.source, event.instance)
	}
}

func (s *Syncer) upsertPool(ctx context.Context, source poolsource.Source, pool domain.ExecutionPool) {
	if err := s.poolRepo.Upsert(ctx, pool); err != nil {
		s.logger.Warn("写入 execution pool 失败",
			elog.String("source", source.Name()),
			elog.String("pool", pool.Name),
			elog.FieldErr(err))
	}
}

func (s *Syncer) disableIfNoInstances(ctx context.Context, source poolsource.Source, inst registry.ServiceInstance) {
	poolName := source.PoolName(inst)
	if poolName == "" {
		return
	}

	hasInstances, ok := s.hasInstances(ctx, source, poolName)
	if !ok {
		return
	}
	if hasInstances {
		s.resyncSource(ctx, source)
		return
	}

	timer := time.NewTimer(disableGracePeriod)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return
	case <-timer.C:
	}

	hasInstances, ok = s.hasInstances(ctx, source, poolName)
	if !ok || hasInstances {
		return
	}
	s.resyncSource(ctx, source)
}

func (s *Syncer) hasInstances(ctx context.Context, source poolsource.Source, poolName string) (bool, bool) {
	hasInstances, err := source.HasInstances(ctx, poolName)
	if err == nil {
		return hasInstances, true
	}
	s.logger.Warn("确认 execution pool 存活实例失败",
		elog.String("source", source.Name()),
		elog.String("pool", poolName),
		elog.FieldErr(err))
	return false, false
}

func (s *Syncer) disableMissing(ctx context.Context, source poolsource.Source, active map[string]struct{}) {
	pools, err := s.poolRepo.ListByKind(ctx, source.Kind())
	if err != nil {
		s.logger.Warn("查询 execution pools 失败",
			elog.String("source", source.Name()),
			elog.String("kind", source.Kind().String()),
			elog.FieldErr(err))
		return
	}

	for _, pool := range pools {
		if !poolsource.IsRegistryManaged(pool) {
			continue
		}
		if _, ok := active[pool.Name]; ok {
			continue
		}
		if pool.Status == domain.ExecutionPoolStatusDisabled {
			continue
		}
		s.disablePool(ctx, pool.Name)
	}
}

func (s *Syncer) disablePool(ctx context.Context, poolName string) {
	if err := s.poolRepo.Disable(ctx, poolName); err != nil {
		s.logger.Warn("禁用 execution pool 失败",
			elog.String("pool", poolName),
			elog.FieldErr(err))
	}
}

func toSourceEvent(source poolsource.Source, etcdEvent *clientv3.Event) (sourceEvent, bool) {
	var kind eventKind
	kv := etcdEvent.Kv

	switch etcdEvent.Type {
	case mvccpb.PUT:
		kind = eventUpsert
	case mvccpb.DELETE:
		kind = eventDelete
		kv = etcdEvent.PrevKv
	default:
		return sourceEvent{}, false
	}

	inst, ok := poolsource.DecodeInstance(kv)
	if !ok || !source.Accept(inst) {
		return sourceEvent{}, false
	}
	return sourceEvent{
		source:   source,
		kind:     kind,
		instance: inst,
	}, true
}
