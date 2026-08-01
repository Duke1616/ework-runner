package pool

import (
	"context"
	"time"

	grpcpkg "github.com/Duke1616/etask/pkg/grpc"
	"github.com/Duke1616/etask/pkg/grpc/registry"
	"github.com/ecodeclub/ekit/syncx"
	"google.golang.org/grpc"
)

type Clients[T any] struct {
	clientMap syncx.Map[string, T]
	registry  registry.Registry
	timeout   time.Duration
	authToken string
	creator   func(conn *grpc.ClientConn) T
}

func NewClients[T any](
	registry registry.Registry,
	timeout time.Duration,
	authToken string,
	creator func(conn *grpc.ClientConn) T,
) *Clients[T] {
	return &Clients[T]{
		registry:  registry,
		timeout:   timeout,
		authToken: authToken,
		creator:   creator,
	}
}

// ListServices 返回客户端池当前服务发现中的实例，用于需要逐节点广播的控制面调用。
func (c *Clients[T]) ListServices(ctx context.Context, serviceName string) ([]registry.ServiceInstance, error) {
	return c.registry.ListServices(ctx, serviceName)
}

// Get 获取带有自定义负载均衡器的客户端
func (c *Clients[T]) Get(serviceName string) T {
	// 尝试加载，如果存在，直接返回
	if client, ok := c.clientMap.Load(serviceName); ok {
		return client
	}

	opts := []grpcpkg.ClientOption{
		grpcpkg.WithServiceName(serviceName),
		grpcpkg.WithTimeout(c.timeout),
	}
	if c.authToken != "" {
		opts = append(opts, grpcpkg.WithClientJWTAuth(c.authToken))
	}

	// 使用封装的函数创建连接
	grpcConn, err := grpcpkg.NewClientConn(c.registry, opts...)
	if err != nil {
		panic(err)
	}

	newClient := c.creator(grpcConn)
	// 使用 LoadOrStore 原子地存储
	// 如果在当前 goroutine 创建期间，有其他 goroutine 已经存入了值，
	// actual 会是那个已经存在的值，ok 会是 true。
	// 这样可以保证我们总是使用第一个被成功创建和存储的 client。
	actual, _ := c.clientMap.LoadOrStore(serviceName, newClient)
	return actual
}
