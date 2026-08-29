package binding

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/Duke1616/etask/internal/pkg/variable"
	"github.com/samber/lo"
)

// ResolveRequest 描述调度中心解析一个参数绑定时使用的上下文。
type ResolveRequest struct {
	// HandlerName 是当前任务使用的 Handler 名称。
	HandlerName string
	// ParamKey 是声明绑定的任务参数键。
	ParamKey string
	// BindingName 是参数元数据中声明的绑定名称。
	BindingName string
	// Value 是任务参数中绑定声明对应的原始值。
	Value string
}

// ResolveResult 将绑定解析结果按输入类型分开，避免敏感变量回到普通参数。
type ResolveResult struct {
	// Parameters 是绑定产生的普通参数值。
	Parameters map[string]string
	// Variables 是绑定产生的结构化变量值。
	Variables []variable.Item
}

// Resolver 将一种绑定类型解析成普通参数或结构化变量。
type Resolver interface {
	// Resolve 根据绑定请求返回解析结果。
	Resolve(ctx context.Context, req ResolveRequest) (ResolveResult, error)
}

// ResolverFunc 将普通函数适配为 Resolver。
type ResolverFunc func(ctx context.Context, req ResolveRequest) (ResolveResult, error)

// Resolve 调用被适配的解析函数。
func (fn ResolverFunc) Resolve(ctx context.Context, req ResolveRequest) (ResolveResult, error) {
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
	name = strings.TrimSpace(name)
	if name == "" || resolver == nil {
		return r
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.resolvers[name] = resolver
	return r
}

// Resolve 按参数名稳定顺序解析已注册的绑定；未注册的绑定会被跳过。
func (r *Registry) Resolve(ctx context.Context, handlerName string, params map[string]string,
	metadata map[string]string) (ResolveResult, error) {
	if r == nil || len(metadata) == 0 {
		return ResolveResult{}, nil
	}

	resolvers := r.resolverSnapshot()
	if len(resolvers) == 0 {
		return ResolveResult{}, nil
	}

	var result ResolveResult
	for _, paramKey := range slices.Sorted(maps.Keys(metadata)) {
		bindingName := strings.TrimSpace(metadata[paramKey])
		if bindingName == "" {
			continue
		}
		resolver, ok := resolvers[bindingName]
		if !ok {
			continue
		}

		resolved, err := resolver.Resolve(ctx, ResolveRequest{
			HandlerName: handlerName,
			ParamKey:    paramKey,
			BindingName: bindingName,
			Value:       params[paramKey],
		})
		if err != nil {
			return ResolveResult{}, fmt.Errorf("解析参数 %s 的 %s 绑定失败: %w", paramKey, bindingName, err)
		}
		if len(resolved.Parameters) > 0 {
			result.Parameters = lo.Assign(result.Parameters, resolved.Parameters)
		}
		result.Variables = append(result.Variables, resolved.Variables...)
	}

	if len(result.Parameters) == 0 && len(result.Variables) == 0 {
		return ResolveResult{}, nil
	}
	return result, nil
}

func (r *Registry) resolverSnapshot() map[string]Resolver {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return maps.Clone(r.resolvers)
}
