package grpc

import (
	"fmt"
	"time"

	"github.com/Duke1616/ecmdb/pkg/grpc/balancer"
	"github.com/Duke1616/ecmdb/pkg/grpc/registry"
	"github.com/ecodeclub/ekit/syncx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Clients[T any] struct {
	clientMap syncx.Map[string, T]
	registry  registry.Registry
	timeout   time.Duration
	creator   func(conn *grpc.ClientConn) T
}

func NewClients[T any](
	registry registry.Registry,
	timeout time.Duration,
	creator func(conn *grpc.ClientConn) T,
) *Clients[T] {
	return &Clients[T]{
		registry: registry,
		timeout:  timeout,
		creator:  creator,
	}
}

// Get 获取带有自定义负载均衡器的客户端
func (c *Clients[T]) Get(serviceName string) T {
	// 尝试加载，如果存在，直接返回
	if client, ok := c.clientMap.Load(serviceName); ok {
		return client
	}

	// 构建带有自定义负载均衡器的连接，如果服务发现失败，会 panic
	grpc.NewServer()
	grpcConn, err := grpc.NewClient(
		fmt.Sprintf("executor:///%s", serviceName),
		// 注入解析器
		grpc.WithResolvers(NewResolverBuilder(c.registry, c.timeout)),
		// 默认负载均衡器实现
		grpc.WithDefaultServiceConfig(fmt.Sprintf(`{"loadBalancingPolicy":%q}`, balancer.RoutingRoundRobinName)),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
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
