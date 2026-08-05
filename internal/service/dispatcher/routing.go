package dispatcher

import (
	"context"
	"fmt"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/service/picker"
	"github.com/Duke1616/etask/pkg/grpc/balancer"
)

//go:generate go tool mockgen -source=./routing.go -package=dispatchermocks -destination=./mocks/routing.mock.go -typed

// Route 描述一次任务派发所需的任务快照和可选目标节点。
type Route struct {
	Task      domain.Task           // 已写入本次派发模式的任务快照。
	Execution domain.ExecutionRoute // 本次执行固定的传输路由。
}

// Context 将路由中的目标节点约束写入派发上下文。
func (r Route) Context(ctx context.Context) context.Context {
	if r.Execution.TargetNodeID == "" {
		return ctx
	}
	return balancer.WithSpecificNodeID(ctx, r.Execution.TargetNodeID)
}

// RoutePlanner 定义任务传输方式和目标节点的规划能力。
type RoutePlanner interface {
	// Plan 根据任务资源池生成派发路由。
	Plan(ctx context.Context, task domain.Task) (Route, error)
}

// ExecutionPoolReader 定义路由规划所需的资源池查询能力。
type ExecutionPoolReader interface {
	// Find 按服务名称查询执行资源池。
	Find(ctx context.Context, name string) (domain.ExecutionPool, error)
}

// routePlanner 根据资源池类型组织 MQ 或 gRPC 路由，不负责执行任务。
type routePlanner struct {
	pools   ExecutionPoolReader
	targets picker.ExecutorNodePicker
}

// NewRoutePlanner 创建任务派发路由规划器。
func NewRoutePlanner(
	pools ExecutionPoolReader,
	targets picker.ExecutorNodePicker,
) RoutePlanner {
	return &routePlanner{pools: pools, targets: targets}
}

// Plan 根据任务资源池生成派发路由。
func (p *routePlanner) Plan(ctx context.Context, task domain.Task) (Route, error) {
	if task.GrpcConfig == nil {
		transport := domain.ExecutionTransportLocal
		if task.HTTPConfig != nil {
			transport = domain.ExecutionTransportHTTP
		}
		task.ExecMode = domain.ExecModePush
		return Route{Task: task, Execution: domain.ExecutionRoute{
			Transport: transport, DispatchMode: domain.ExecModePush,
		}}, nil
	}
	pool, err := p.pools.Find(ctx, task.GrpcConfig.ServiceName)
	if err != nil {
		return Route{}, fmt.Errorf("查询执行资源池失败: %w", err)
	}
	switch pool.Transport {
	case domain.ExecutionTransportMQ:
		task.ExecMode = domain.ExecModePush
		route := domain.ExecutionRoute{
			Transport: domain.ExecutionTransportMQ, DispatchMode: domain.ExecModePush,
			PoolName: pool.Name, Topic: pool.Metadata["topic"],
		}
		if err = route.Validate(); err != nil {
			return Route{}, err
		}
		return Route{Task: task, Execution: route}, nil
	case domain.ExecutionTransportGRPC:
		return p.grpcRoute(ctx, task, pool)
	default:
		return Route{}, fmt.Errorf("执行资源池 %s 传输通道非法: %s", pool.Name, pool.Transport)
	}
}

// grpcRoute 使用资源池快照决定派发模式；只有 PUSH 需要提前选择目标节点。
func (p *routePlanner) grpcRoute(ctx context.Context, task domain.Task, pool domain.ExecutionPool) (Route, error) {
	task.ExecMode = pool.DispatchMode
	nodeID := ""
	if pool.DispatchMode.IsPush() {
		var err error
		nodeID, err = p.targets.Pick(ctx, task)
		if err != nil {
			return Route{}, fmt.Errorf("选择 gRPC 执行目标失败: %w", err)
		}
	}
	route := domain.ExecutionRoute{
		Transport: domain.ExecutionTransportGRPC, DispatchMode: pool.DispatchMode,
		PoolName: pool.Name, TargetNodeID: nodeID,
	}
	if err := route.Validate(); err != nil {
		return Route{}, err
	}
	return Route{Task: task, Execution: route}, nil
}
