package binding

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sync"
)

// ResolveRequest 描述调度中心解析一个参数绑定时使用的上下文。
type ResolveRequest struct {
	HandlerName string
	ParamKey    string
	BindingName string
	Value       string
	Params      map[string]string
	Metadata    map[string]string
}

// Resolver 将一种绑定类型解析成发送给执行器的参数值。
type Resolver interface {
	Resolve(ctx context.Context, req ResolveRequest) (string, error)
}

// ResolverFunc 将普通函数适配为 Resolver。
type ResolverFunc func(ctx context.Context, req ResolveRequest) (string, error)

// Resolve 调用被适配的解析函数。
func (fn ResolverFunc) Resolve(ctx context.Context, req ResolveRequest) (string, error) {
	return fn(ctx, req)
}

// Registry 保存调度中心使用的参数绑定解析器。
type Registry struct {
	mu        sync.RWMutex
	resolvers map[string]Resolver
}

// NewRegistry 创建一个空的解析器注册表。
func NewRegistry() *Registry {
	return &Registry{resolvers: make(map[string]Resolver)}
}

// Register 按绑定名称注册或替换解析器。
func (r *Registry) Register(name string, resolver Resolver) *Registry {
	if name == "" || resolver == nil {
		return r
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.resolvers[name] = resolver
	return r
}

// Resolve 按参数名稳定顺序解析已注册的绑定。
// 未注册的绑定会被跳过，传入的参数和元数据不会被修改。
func (r *Registry) Resolve(ctx context.Context, handlerName string, params map[string]string,
	metadata map[string]string) (map[string]string, error) {
	if r == nil || len(metadata) == 0 {
		return nil, nil
	}

	resolvers := r.resolverSnapshot()
	if len(resolvers) == 0 {
		return nil, nil
	}

	paramsView := maps.Clone(params)
	metadataView := maps.Clone(metadata)
	resolved := make(map[string]string, len(metadataView))
	for _, paramKey := range slices.Sorted(maps.Keys(metadataView)) {
		bindingName := metadataView[paramKey]
		resolver, ok := resolvers[bindingName]
		if !ok {
			continue
		}

		value, err := resolver.Resolve(ctx, ResolveRequest{
			HandlerName: handlerName,
			ParamKey:    paramKey,
			BindingName: bindingName,
			Value:       paramsView[paramKey],
			Params:      paramsView,
			Metadata:    metadataView,
		})
		if err != nil {
			return nil, fmt.Errorf("解析参数 %s 的 %s 绑定失败: %w", paramKey, bindingName, err)
		}
		resolved[paramKey] = value
	}

	if len(resolved) == 0 {
		return nil, nil
	}
	return resolved, nil
}

func (r *Registry) resolverSnapshot() map[string]Resolver {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return maps.Clone(r.resolvers)
}
