package etcd

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Duke1616/etask/pkg/grpc/registry"
	"github.com/gotomicro/ego/core/elog"
	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
)

const (
	// DefaultPrefix 默认服务注册前缀
	DefaultPrefix = "/grpc/services"
)

var typesMap = map[mvccpb.Event_EventType]registry.EventType{
	mvccpb.PUT:    registry.EventTypeAdd,
	mvccpb.DELETE: registry.EventTypeDelete,
}

type Registry struct {
	client   *clientv3.Client
	prefix   string // 服务注册前缀
	indexers []registry.Indexer

	mutex   sync.RWMutex
	cancels map[string]func() // 统一管理所有异步任务的生命周期
	watchID atomic.Uint64
	logger  *elog.Component
}

// NewRegistry 创建 Registry,使用默认前缀
func NewRegistry(c *clientv3.Client) (*Registry, error) {
	return NewRegistryWithPrefix(c, DefaultPrefix)
}

// NewRegistryWithPrefix 创建 Registry,使用自定义前缀
func NewRegistryWithPrefix(c *clientv3.Client, prefix string) (*Registry, error) {
	return &Registry{
		client:  c,
		prefix:  prefix,
		cancels: make(map[string]func()),
		logger:  elog.DefaultLogger.With(elog.FieldComponentName("grpc.registry.etcd")),
	}, nil
}

func (r *Registry) UseIndexers(indexers ...registry.Indexer) *Registry {
	r.indexers = append(r.indexers, indexers...)
	return r
}

// unmarshal 统一解析 etcd 节点数据
func (r *Registry) unmarshal(kv *mvccpb.KeyValue) (registry.ServiceInstance, bool) {
	var si registry.ServiceInstance
	if kv == nil || len(kv.Value) == 0 {
		return si, false
	}
	if err := json.Unmarshal(kv.Value, &si); err != nil {
		r.logger.Warn("解析实例数据失败", elog.String("key", string(kv.Key)), elog.FieldErr(err))
		return si, false
	}
	return si, true
}

func (r *Registry) Register(ctx context.Context, si registry.ServiceInstance) error {
	val, err := json.Marshal(si)
	if err != nil {
		return err
	}

	key := r.instanceKey(si)
	regCtx, cancel := context.WithCancel(context.Background())
	r.registerCancel(key, cancel)

	go r.keepAlive(regCtx, si, string(val))
	return nil
}

// keepAlive 核心维护逻辑：建立 Session -> Put 数据 -> 阻塞等待失效 -> 失败重试
func (r *Registry) keepAlive(ctx context.Context, si registry.ServiceInstance, val string) {
	backoff := time.Second
	for {
		// 1. 建立 Session
		sess, err := concurrency.NewSession(r.client, concurrency.WithTTL(30))
		if err == nil {
			// 2. 注册服务
			_, err = r.client.Put(ctx, r.instanceKey(si), val, clientv3.WithLease(sess.Lease()))
			if err == nil {
				err = r.putIndexes(ctx, sess, si, val)
			}
		}

		// 3. 失败处理：退避重试
		if err != nil {
			if sess != nil {
				_ = sess.Close()
			}
			r.logger.Error("服务注册/全量同步失败", elog.FieldErr(err), elog.String("backoff", backoff.String()))
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
				backoff = min(backoff*2, 30*time.Second)
				continue
			}
		}

		// 4. 成功后阻塞：重置退避时间，等待 Session 挂掉或被取消
		backoff = time.Second
		select {
		case <-ctx.Done():
			_ = sess.Close()
			return
		case <-sess.Done():
			r.logger.Warn("Session 失效，准备重新注册", elog.String("instance", si.Address))
		}
	}
}

func (r *Registry) instanceKey(s registry.ServiceInstance) string {
	return fmt.Sprintf("%s/%s/%s", r.prefix, s.Name, s.Address)
}

func (r *Registry) putIndexes(ctx context.Context, sess *concurrency.Session, si registry.ServiceInstance, val string) error {
	for _, indexer := range r.indexers {
		key, ok := indexer.Key(si)
		if !ok {
			continue
		}
		if _, err := r.client.Put(ctx, key, val, clientv3.WithLease(sess.Lease())); err != nil {
			return err
		}
	}
	return nil
}

func (r *Registry) UnRegister(ctx context.Context, si registry.ServiceInstance) error {
	key := r.instanceKey(si)
	r.mutex.Lock()
	if cancel, ok := r.cancels[key]; ok {
		cancel()
		delete(r.cancels, key)
	}
	r.mutex.Unlock()

	if _, err := r.client.Delete(ctx, key); err != nil {
		return err
	}
	return r.deleteIndexes(ctx, si)
}

func (r *Registry) deleteIndexes(ctx context.Context, si registry.ServiceInstance) error {
	for _, indexer := range r.indexers {
		key, ok := indexer.Key(si)
		if !ok {
			continue
		}
		if _, err := r.client.Delete(ctx, key); err != nil {
			return err
		}
	}
	return nil
}

func (r *Registry) ListServices(ctx context.Context, name string) ([]registry.ServiceInstance, error) {
	key := r.serviceKey(name)
	resp, err := r.client.Get(ctx, key, clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}

	res := make([]registry.ServiceInstance, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		if si, ok := r.unmarshal(kv); ok {
			res = append(res, si)
		}
	}
	return res, nil
}

func (r *Registry) serviceKey(name string) string {
	// WithPrefix 必须以目录分隔符结尾，避免 foo 匹配到 foobar 服务。
	return fmt.Sprintf("%s/%s/", r.prefix, name)
}

func (r *Registry) Subscribe(name string) <-chan registry.Event {
	return r.SubscribeContext(context.Background(), name)
}

// SubscribeContext 订阅服务变更，并在 ctx 取消时释放 etcd watcher。
func (r *Registry) SubscribeContext(parent context.Context, name string) <-chan registry.Event {
	res := make(chan registry.Event, 64)
	ctx, cancel := context.WithCancel(parent)
	key := fmt.Sprintf("%s_watch_%d", name, r.watchID.Add(1))
	r.registerCancel(key, cancel)

	go func() {
		defer close(res)
		defer r.removeCancel(key)
		defer cancel()
		for {
			// 使用 for-range 自动处理 channel 接收逻辑
			ch := r.client.Watch(ctx, r.serviceKey(name), clientv3.WithPrefix(), clientv3.WithPrevKV())
			for resp := range ch {
				if resp.Err() != nil {
					break // 发生错误延迟重练
				}

				for _, event := range resp.Events {
					kv := event.Kv
					if event.Type == mvccpb.DELETE {
						kv = event.PrevKv
					}

					if si, ok := r.unmarshal(kv); ok {
						select {
						case res <- registry.Event{Type: typesMap[event.Type], Instance: si}:
						case <-ctx.Done():
							return
						}
					}
				}
			}

			// Watch 异常断开，延迟一秒重刷
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
		}
	}()
	return res
}

func (r *Registry) removeCancel(key string) {
	r.mutex.Lock()
	delete(r.cancels, key)
	r.mutex.Unlock()
}

// registerCancel 统一管理异步任务的取消函数
func (r *Registry) registerCancel(key string, cancel func()) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if r.cancels == nil {
		r.cancels = make(map[string]func())
	}
	if old, ok := r.cancels[key]; ok {
		old()
	}
	r.cancels[key] = cancel
}

func (r *Registry) Close() error {
	r.mutex.Lock()
	for _, cancel := range r.cancels {
		cancel()
	}
	r.cancels = make(map[string]func())
	r.mutex.Unlock()
	return nil
}
