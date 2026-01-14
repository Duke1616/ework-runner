package invoker

import (
	"context"
	"fmt"

	executorv1 "github.com/Duke1616/ecmdb/api/proto/gen/executor/v1"
	"github.com/Duke1616/ecmdb/internal/domain"
	"github.com/Duke1616/ecmdb/pkg/grpc"
	"github.com/gotomicro/ego/core/elog"
)

var _ Invoker = &GRPCInvoker{}

// GRPCInvoker 远程执行器
type GRPCInvoker struct {
	grpcClients *grpc.Clients[executorv1.ExecutorServiceClient] // gRPC客户端池
	logger      *elog.Component
}

// NewGRPCInvoker 创建 GRPCInvoker 实例
func NewGRPCInvoker(
	grpcClients *grpc.Clients[executorv1.ExecutorServiceClient],
) *GRPCInvoker {
	return &GRPCInvoker{
		grpcClients: grpcClients,
		logger:      elog.DefaultLogger.With(elog.FieldComponentName("executor.GRPCInvoker")),
	}
}

func (r *GRPCInvoker) Name() string {
	return "GRPC"
}

func (r *GRPCInvoker) Run(ctx context.Context, exec domain.TaskExecution) (domain.ExecutionState, error) {
	client := r.grpcClients.Get(exec.Task.GrpcConfig.ServiceName)
	// 发送执行请求

	resp, err := client.Execute(ctx, &executorv1.ExecuteRequest{
		Eid:      exec.ID,
		TaskId:   exec.Task.ID,
		TaskName: exec.Task.Name,
		Params:   exec.GRPCParams(),
	})
	if err != nil {
		return domain.ExecutionState{}, fmt.Errorf("发送gRPC请求失败: %w", err)
	}
	return domain.ExecutionStateFromProto(resp.GetExecutionState()), nil
}

func (r *GRPCInvoker) Prepare(ctx context.Context, exec domain.TaskExecution) (map[string]string, error) {
	client := r.grpcClients.Get(exec.Task.GrpcConfig.ServiceName)
	// 发送执行请求
	resp, err := client.Prepare(ctx, &executorv1.PrepareRequest{
		Eid:      exec.ID,
		TaskId:   exec.Task.ID,
		TaskName: exec.Task.Name,
		Params:   exec.GRPCParams(),
	})
	if err != nil {
		return nil, fmt.Errorf("发送gRPC请求失败: %w", err)
	}
	return resp.GetParams(), nil
}
