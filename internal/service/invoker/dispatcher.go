package invoker

import (
	"context"

	"github.com/Duke1616/etask/internal/domain"
)

var _ Invoker = &Dispatcher{}

type Dispatcher struct {
	http  *HTTPInvoker
	grpc  *GRPCInvoker
	local *LocalInvoker
	mq    *MQInvoker
}

func NewDispatcher(http *HTTPInvoker, grpc *GRPCInvoker, local *LocalInvoker,
	mq *MQInvoker) *Dispatcher {
	return &Dispatcher{
		http: http, grpc: grpc, local: local, mq: mq,
	}
}

func (r *Dispatcher) Name() string {
	return "dispatcher"
}

func (r *Dispatcher) Run(ctx context.Context, execution domain.TaskExecution) (domain.ExecutionState, error) {
	// 传输通道已经在创建执行记录时固定，调用阶段不再读取动态资源池。
	if err := execution.Route.Validate(); err != nil {
		return domain.ExecutionState{}, err
	}
	switch execution.Route.Transport {
	case domain.ExecutionTransportGRPC:
		return r.grpc.Run(ctx, execution)
	case domain.ExecutionTransportMQ:
		return r.mq.Run(ctx, execution, execution.Route.Topic)
	case domain.ExecutionTransportHTTP:
		return r.http.Run(ctx, execution)
	case domain.ExecutionTransportLocal:
		return r.local.Run(ctx, execution)
	default:
		return domain.ExecutionState{}, execution.Route.Validate()
	}
}

func (r *Dispatcher) Terminate(ctx context.Context, execution domain.TaskExecution, reason string) error {
	if err := execution.Route.Validate(); err != nil {
		return err
	}
	switch execution.Route.Transport {
	case domain.ExecutionTransportGRPC:
		return r.grpc.Terminate(ctx, execution, reason)
	case domain.ExecutionTransportMQ:
		return r.mq.Terminate(ctx, execution, reason)
	case domain.ExecutionTransportHTTP, domain.ExecutionTransportLocal:
		// 这两种同步调用没有独立控制面；数据库 CANCELLED 终态仍会屏蔽晚到结果。
		return nil
	default:
		return execution.Route.Validate()
	}
}
