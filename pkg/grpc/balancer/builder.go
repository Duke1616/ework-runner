package balancer

import (
	"google.golang.org/grpc/balancer"
)

// routingBalancerBuilder 实现 BalancerBuilder 接口
type routingBalancerBuilder struct{}

// Build 创建新的负载均衡器实例
func (b *routingBalancerBuilder) Build(cc balancer.ClientConn, _ balancer.BuildOptions) balancer.Balancer {
	return newRoutingBalancer(cc)
}

// Name 返回负载均衡器的名称
func (b *routingBalancerBuilder) Name() string {
	return RoutingRoundRobinName
}

// init 函数在包初始化时注册负载均衡器
func init() {
	balancer.Register(&routingBalancerBuilder{})
}
