package task

// 本文件实现并发安全的 Handler 注册中心。

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"sync"

	"github.com/samber/lo"
)

// HandlerRegistry 处理器注册中心，由于 Executor 和 Agent Service 共用
type HandlerRegistry struct {
	mu       sync.RWMutex
	handlers map[string]TaskHandler
}

func NewHandlerRegistry() *HandlerRegistry {
	return &HandlerRegistry{
		handlers: make(map[string]TaskHandler),
	}
}

// Register 注册处理器并在输入非法或名称冲突时返回错误。
func (r *HandlerRegistry) Register(handlers ...TaskHandler) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	type registration struct {
		name    string
		handler TaskHandler
	}
	names := make(map[string]struct{}, len(handlers))
	validated := make([]registration, 0, len(handlers))
	for _, h := range handlers {
		if h == nil {
			return fmt.Errorf("任务处理器不能为空")
		}
		name := h.Name()
		if name == "" {
			return fmt.Errorf("任务处理器名称不能为空")
		}
		if _, exists := r.handlers[name]; exists {
			return fmt.Errorf("任务处理器名称重复: %s", name)
		}
		if _, exists := names[name]; exists {
			return fmt.Errorf("任务处理器名称重复: %s", name)
		}
		names[name] = struct{}{}
		validated = append(validated, registration{name: name, handler: h})
	}
	for _, value := range validated {
		r.handlers[value.name] = value.handler
	}
	return nil
}

// Get 根据名称获取处理器
func (r *HandlerRegistry) Get(name string) (TaskHandler, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	h, ok := r.handlers[name]
	return h, ok
}

// ListMetas 返回所有处理器的元数据清单 (用于上报、展示)
func (r *HandlerRegistry) ListMetas() []HandlerMeta {
	handlers := r.Snapshot()
	metas := lo.Map(lo.Values(handlers), func(h TaskHandler, _ int) HandlerMeta {
		meta := HandlerMeta{
			Name:     h.Name(),
			Desc:     h.Desc(),
			Metadata: h.Metadata(),
		}
		if programHandler, ok := h.(ProgramHandler); ok {
			meta.ProgramKinds = slices.Clone(programHandler.ProgramKinds())
		}
		return meta
	})
	sort.Slice(metas, func(i, j int) bool { return metas[i].Name < metas[j].Name })
	return metas
}

// Names 返回按名称排序的处理器名称列表。
func (r *HandlerRegistry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := lo.Keys(r.handlers)
	sort.Strings(names)
	return names
}

// Snapshot 返回处理器映射副本，调用方无法修改注册中心内部状态。
func (r *HandlerRegistry) Snapshot() map[string]TaskHandler {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return maps.Clone(r.handlers)
}
