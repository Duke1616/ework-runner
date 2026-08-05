package pool

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	grpcpkg "github.com/Duke1616/etask/pkg/grpc"
	"github.com/Duke1616/etask/pkg/grpc/registry"
	"google.golang.org/grpc"
)

type Clients[T any] struct {
	clients   map[string]clientEntry[T]
	registry  registry.Registry
	timeout   time.Duration
	authToken string
	creator   func(conn *grpc.ClientConn) T
	mu        sync.Mutex
	closed    bool
}

type clientEntry[T any] struct {
	client T
	conn   *grpc.ClientConn
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
		clients:   make(map[string]clientEntry[T]),
	}
}

// ListServices 返回客户端池当前服务发现中的实例，用于需要逐节点广播的控制面调用。
func (c *Clients[T]) ListServices(ctx context.Context, serviceName string) ([]registry.ServiceInstance, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, fmt.Errorf("grpc client pool is closed")
	}
	reg := c.registry
	c.mu.Unlock()
	return reg.ListServices(ctx, serviceName)
}

// Get 获取带有自定义负载均衡器的客户端
func (c *Clients[T]) Get(serviceName string) T {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		panic("grpc client pool is closed")
	}
	if serviceName == "" {
		panic("grpc service name is required")
	}
	// 尝试加载，如果存在，直接返回
	if entry, ok := c.clients[serviceName]; ok {
		return entry.client
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

	entry := clientEntry[T]{client: c.creator(grpcConn), conn: grpcConn}
	c.clients[serviceName] = entry
	return entry.client
}

// Close 关闭池持有的全部底层连接。Close 可以重复调用。
func (c *Clients[T]) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	var closeErrors []error
	for serviceName, entry := range c.clients {
		if err := entry.conn.Close(); err != nil {
			closeErrors = append(closeErrors, fmt.Errorf("关闭 gRPC 服务 %s 的连接失败: %w", serviceName, err))
		}
		delete(c.clients, serviceName)
	}
	return errors.Join(closeErrors...)
}
