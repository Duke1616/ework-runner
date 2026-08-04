package binding

import (
	"context"
	"fmt"
	"maps"
	"sort"
	"sync"
)

// ResolveRequest describes one scheduler-side parameter binding request.
type ResolveRequest struct {
	HandlerName string
	ParamKey    string
	BindingName string
	Value       string
	Params      map[string]string
	Metadata    map[string]string
}

// Resolver resolves one binding type into the value sent to an Executor.
type Resolver interface {
	Resolve(ctx context.Context, req ResolveRequest) (string, error)
}

// ResolverFunc adapts a function to Resolver.
type ResolverFunc func(ctx context.Context, req ResolveRequest) (string, error)

// Resolve calls the adapted function.
func (fn ResolverFunc) Resolve(ctx context.Context, req ResolveRequest) (string, error) {
	return fn(ctx, req)
}

// Registry stores scheduler-side parameter resolvers.
type Registry struct {
	mu        sync.RWMutex
	resolvers map[string]Resolver
}

// NewRegistry creates an empty resolver registry.
func NewRegistry() *Registry {
	return &Registry{resolvers: make(map[string]Resolver)}
}

// Register registers or replaces a resolver by binding name.
func (r *Registry) Register(name string, resolver Resolver) *Registry {
	if name == "" || resolver == nil {
		return r
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.resolvers[name] = resolver
	return r
}

// Resolve resolves registered bindings in stable parameter-name order.
func (r *Registry) Resolve(ctx context.Context, handlerName string, params map[string]string,
	metadata map[string]string) (map[string]string, error) {
	if r == nil || len(metadata) == 0 {
		return nil, nil
	}

	resolvers := r.resolverSnapshot()
	if len(resolvers) == 0 {
		return nil, nil
	}

	paramsSnapshot := maps.Clone(params)
	metadataSnapshot := maps.Clone(metadata)
	resolved := make(map[string]string, len(metadataSnapshot))
	keys := make([]string, 0, len(metadataSnapshot))
	for paramKey := range metadataSnapshot {
		keys = append(keys, paramKey)
	}
	sort.Strings(keys)
	for _, paramKey := range keys {
		bindingName := metadataSnapshot[paramKey]
		resolver, ok := resolvers[bindingName]
		if !ok {
			continue
		}

		value, err := resolver.Resolve(ctx, ResolveRequest{
			HandlerName: handlerName,
			ParamKey:    paramKey,
			BindingName: bindingName,
			Value:       paramsSnapshot[paramKey],
			Params:      paramsSnapshot,
			Metadata:    metadataSnapshot,
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
