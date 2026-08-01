package runtime

// 本文件实现 Executor gRPC 服务接口。

import (
	"context"

	executorv1 "github.com/Duke1616/etask/api/proto/gen/etask/executor/v1"
)

// Execute 接收任务并异步启动执行。
func (e *Executor) Execute(ctx context.Context, req *executorv1.ExecuteRequest) (*executorv1.ExecuteResponse, error) {
	return e.startExecution(ctx, req)
}

// Query 查询当前节点内保存的执行状态。
func (e *Executor) Query(_ context.Context, req *executorv1.QueryRequest) (*executorv1.QueryResponse, error) {
	eid := req.GetEid()
	if state, exists := e.executions.Get(eid); exists {
		return &executorv1.QueryResponse{ExecutionState: state}, nil
	}
	return &executorv1.QueryResponse{ExecutionState: &executorv1.ExecutionState{
		Id: eid, Status: executorv1.ExecutionStatus_UNKNOWN,
	}}, nil
}

// Interrupt 请求取消一个正在运行的任务。
func (e *Executor) Interrupt(_ context.Context, req *executorv1.InterruptRequest) (*executorv1.InterruptResponse, error) {
	state, exists := e.executions.Cancel(req.GetEid())
	if !exists {
		return &executorv1.InterruptResponse{Success: false}, nil
	}
	return &executorv1.InterruptResponse{Success: true, ExecutionState: state}, nil
}

// Terminate 强制终止任务并立即返回 CANCELLED 本地状态。
func (e *Executor) Terminate(_ context.Context,
	req *executorv1.TerminateRequest) (*executorv1.TerminateResponse, error) {
	state, exists := e.executions.Terminate(req.GetEid(), req.GetReason())
	if !exists {
		return &executorv1.TerminateResponse{Success: false}, nil
	}
	return &executorv1.TerminateResponse{Success: true, ExecutionState: state}, nil
}
