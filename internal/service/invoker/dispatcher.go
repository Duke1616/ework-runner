package invoker

import (
	"context"

	"github.com/Duke1616/ework-runner/internal/domain"
)

var _ Invoker = &Dispatcher{}

type Dispatcher struct {
	http  *HTTPInvoker
	grpc  *GRPCInvoker
	local *LocalInvoker
}

func NewDispatcher(http *HTTPInvoker, grpc *GRPCInvoker, local *LocalInvoker) *Dispatcher {
	return &Dispatcher{
		http:  http,
		grpc:  grpc,
		local: local,
	}
}

func (r *Dispatcher) Name() string {
	return "dispatcher"
}

func (r *Dispatcher) Run(ctx context.Context, execution domain.TaskExecution) (domain.ExecutionState, error) {
	// 根据配置发送执行请求
	switch {
	case execution.Task.GrpcConfig != nil:
		return r.grpc.Run(ctx, execution)
	case execution.Task.HTTPConfig != nil:
		return r.http.Run(ctx, execution)
	default:
		// 都没有就假定是本地的了
		return r.local.Run(ctx, execution)
	}
}

func (r *Dispatcher) Prepare(ctx context.Context, execution domain.TaskExecution) (map[string]string, error) {
	// 根据配置发送执行请求
	switch {
	case execution.Task.GrpcConfig != nil:
		return r.grpc.Prepare(ctx, execution)
	case execution.Task.HTTPConfig != nil:
		return r.http.Prepare(ctx, execution)
	default:
		// 都没有就假定是本地的了
		return r.local.Prepare(ctx, execution)
	}
}
