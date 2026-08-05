package registry

import (
	"context"
	"io"
)

type Registry interface {
	// Register 注册服务实例。
	Register(ctx context.Context, si ServiceInstance) error
	// UnRegister 注销服务实例。
	UnRegister(ctx context.Context, si ServiceInstance) error

	// ListServices 查询指定名称的服务实例。
	ListServices(ctx context.Context, name string) ([]ServiceInstance, error)
	// Subscribe 订阅指定服务的注册变更事件。
	Subscribe(name string) <-chan Event

	io.Closer
}

// ContextSubscriber 支持由调用方控制订阅生命周期。
// Registry 的第三方实现可以选择实现该接口；resolver 会优先使用它。
type ContextSubscriber interface {
	SubscribeContext(ctx context.Context, name string) <-chan Event
}

type Indexer interface {
	// Name 返回索引器名称。
	Name() string
	// Key 返回服务实例的索引键以及是否应该建立索引。
	Key(si ServiceInstance) (string, bool)
}

type ServiceInstance struct {
	Name         string // im
	Address      string // 1.1.0.129
	ID           string // 01
	Weight       int64
	InitCapacity int64
	MaxCapacity  int64
	IncreaseStep int64
	GrowthRate   float64
	Metadata     map[string]any // 存储附加的元数据信息
}

type EventType int

const (
	EventTypeUnknown EventType = iota
	EventTypeAdd
	EventTypeDelete
)

func (e EventType) IsAdd() bool {
	return e == EventTypeAdd
}

func (e EventType) IsDelete() bool {
	return e == EventTypeDelete
}

type Event struct {
	Type     EventType
	Instance ServiceInstance
}
