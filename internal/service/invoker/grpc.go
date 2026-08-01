package invoker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	executorv1 "github.com/Duke1616/etask/api/proto/gen/etask/executor/v1"
	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/pkg/grpc/balancer"
	"github.com/Duke1616/etask/pkg/grpc/pool"
	"github.com/gotomicro/ego/core/elog"
)

var _ Invoker = &GRPCInvoker{}

// GRPCInvoker 远程执行器
type GRPCInvoker struct {
	grpcClients *pool.Clients[executorv1.ExecutorServiceClient] // gRPC客户端池
	logger      *elog.Component
}

// NewGRPCInvoker 创建 GRPCInvoker 实例
func NewGRPCInvoker(
	grpcClients *pool.Clients[executorv1.ExecutorServiceClient],
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
	artifacts, err := domain.ArtifactRefsToProto(exec.Artifacts)
	if err != nil {
		return domain.ExecutionState{}, fmt.Errorf("执行制品引用非法: %w", err)
	}
	// 获取 client
	client := r.grpcClients.Get(exec.Task.GrpcConfig.ServiceName)

	// 设置调用超时(30秒), 防止无 executor 节点时无限等待
	callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := client.Execute(callCtx, &executorv1.ExecuteRequest{
		Eid:             exec.ID,
		TaskId:          exec.Task.ID,
		TaskName:        exec.Task.Name,
		TaskHandlerName: exec.Task.GrpcConfig.HandlerName,
		Params:          exec.GRPCParams(),
		TenantId:        exec.Task.TenantID,
		Artifacts:       artifacts,
	})

	if err != nil {
		return domain.ExecutionState{}, fmt.Errorf("发送gRPC请求失败: %w", err)
	}

	return domain.ExecutionStateFromProto(resp.GetExecutionState()), nil
}

func (r *GRPCInvoker) Terminate(ctx context.Context, exec domain.TaskExecution, reason string) error {
	if exec.Task.GrpcConfig == nil {
		return nil
	}
	nodeIDs := []string{exec.ExecutorNodeID}
	if exec.ExecutorNodeID == "" {
		instances, err := r.grpcClients.ListServices(ctx, exec.Task.GrpcConfig.ServiceName)
		if err != nil {
			return fmt.Errorf("查询 Executor 实例失败: %w", err)
		}
		nodeIDs = nodeIDs[:0]
		seen := make(map[string]struct{}, len(instances))
		for _, instance := range instances {
			if instance.ID == "" {
				continue
			}
			if _, exists := seen[instance.ID]; exists {
				continue
			}
			seen[instance.ID] = struct{}{}
			nodeIDs = append(nodeIDs, instance.ID)
		}
	}
	if len(nodeIDs) == 0 {
		return nil
	}
	client := r.grpcClients.Get(exec.Task.GrpcConfig.ServiceName)
	callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	var mu sync.Mutex
	terminateErrors := make([]error, 0)
	for _, nodeID := range nodeIDs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			nodeCtx := balancer.WithSpecificNodeID(callCtx, nodeID)
			if _, err := client.Terminate(nodeCtx, &executorv1.TerminateRequest{
				Eid: exec.ID, Reason: reason,
			}); err != nil {
				mu.Lock()
				terminateErrors = append(terminateErrors,
					fmt.Errorf("终止 Executor 节点 %s 失败: %w", nodeID, err))
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return errors.Join(terminateErrors...)
}
