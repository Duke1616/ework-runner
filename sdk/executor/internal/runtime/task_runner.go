package runtime

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Duke1616/eiam/pkg/ctxutil"
	executorv1 "github.com/Duke1616/etask/api/proto/gen/etask/executor/v1"
	reporterv1 "github.com/Duke1616/etask/api/proto/gen/etask/reporter/v1"
	"github.com/Duke1616/etask/pkg/grpc/interceptors/tenant"
	enginepkg "github.com/Duke1616/etask/sdk/executor/internal/engine"
	"github.com/Duke1616/etask/sdk/executor/internal/execution"
	"github.com/Duke1616/etask/sdk/executor/internal/task"
	"github.com/gotomicro/ego/core/elog"
	"google.golang.org/grpc"
)

const finalReportTimeout = 10 * time.Second

func (e *Executor) startExecution(ctx context.Context, req *executorv1.ExecuteRequest) (*executorv1.ExecuteResponse, error) {
	eid := req.GetEid()
	if e.executionClient != nil {
		response, err := e.executionClient.GetTaskExecution(ctx, &executorv1.GetTaskExecutionRequest{
			ExecutionId: eid,
		})
		if err != nil {
			return nil, fmt.Errorf("启动前查询 execution 状态失败: %w", err)
		}
		status := response.GetExecution().GetStatus()
		if status == executorv1.ExecutionStatus_CANCELLED {
			state, _ := e.executions.Terminate(eid, response.GetExecution().GetTaskResult())
			return &executorv1.ExecuteResponse{ExecutionState: state}, nil
		}
		if status == executorv1.ExecutionStatus_SUCCESS || status == executorv1.ExecutionStatus_FAILED {
			return &executorv1.ExecuteResponse{ExecutionState: &executorv1.ExecutionState{
				Id: eid, TaskId: response.GetExecution().GetTaskId(),
				TaskName: response.GetExecution().GetTaskName(), Status: status,
				TaskResult: response.GetExecution().GetTaskResult(),
			}}, nil
		}
	}
	// 执行上下文脱离单次 RPC 生命周期，但保留租户并允许 Interrupt 主动取消。
	runCtx, cancel := context.WithCancelCause(executionContext(ctx, req.GetTenantId()))
	e.runMu.Lock()
	if e.stopping {
		e.runMu.Unlock()
		cancel(nil)
		return nil, fmt.Errorf("executor 正在停止，不再接收新任务")
	}
	// Begin 同时承担幂等保护，相同执行 ID 正在运行时直接返回已有状态。
	state, started := e.executions.Begin(initialState(req, e.config.Server.ServiceId), cancel)
	if !started {
		e.runMu.Unlock()
		cancel(nil)
		e.logger.Warn("任务已在执行中", elog.Int64("eid", eid))
		return &executorv1.ExecuteResponse{ExecutionState: state}, nil
	}
	e.runWG.Add(1)
	e.runMu.Unlock()
	e.logger.Info("启动异步任务执行", elog.Int64("eid", eid))
	go func() {
		defer e.runWG.Done()
		defer cancel(nil)
		e.runTask(runCtx, req)
	}()
	return &executorv1.ExecuteResponse{ExecutionState: state}, nil
}

func (e *Executor) runTask(ctx context.Context, req *executorv1.ExecuteRequest) {
	executionID := req.GetEid()
	logger := e.logger.With(elog.Int64("executionID", executionID), elog.Int64("taskID", req.GetTaskId()))
	// Handler 属于扩展代码，panic 必须在执行边界收敛并转成可上报的失败状态。
	defer func() {
		if recovered := recover(); recovered != nil {
			err := fmt.Errorf("任务处理器发生 panic: %v", recovered)
			logger.Error("任务执行异常", elog.FieldErr(err))
			e.reportFinalResult(ctx, executionID, executorv1.ExecutionStatus_FAILED, err.Error())
		}
	}()
	// Engine 统一完成制品准备、任务 Context 创建、Handler 调用和现场回收。
	result, err := e.engine.Execute(ctx, enginepkg.Command{
		Task: task.TaskInfo{
			ExecutionID: executionID, TaskID: req.GetTaskId(),
			Name: req.GetTaskName(), Handler: req.GetTaskHandlerName(),
			ExecutorNodeID: e.config.Server.ServiceId,
		},
		Params: req.GetParams(), Parameters: e.handlerMetadata(req.GetTaskHandlerName()),
		Artifacts: artifactRefs(req.GetArtifacts()), TaskLogger: e.newTaskLogger(ctx, executionID),
	})
	e.finishTask(ctx, executionID, result.Value, logger, err)
}

func (e *Executor) finishTask(ctx context.Context, executionID int64, result string,
	logger *elog.Component, err error) {
	status := resolveFinalStatus(ctx, err)
	logFinalStatus(logger, status, err)
	if err != nil && result == "" {
		result = err.Error()
	}
	e.reportFinalResult(ctx, executionID, status, result)
}

func (e *Executor) handlerMetadata(name string) []task.Parameter {
	handler, exists := e.hr.Get(name)
	if !exists {
		return nil
	}
	return handler.Metadata()
}

func (e *Executor) reportFinalResult(ctx context.Context, eid int64, status executorv1.ExecutionStatus, result string) {
	// 先原子落本地终态，确保 Query 与重复上报读取到同一份结果。
	state, exists := e.executions.Finish(eid, status, result)
	if !exists || e.reporterClient == nil {
		return
	}
	// 最终上报不能继承任务取消信号；短暂断连时等待连接恢复。
	reportCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), finalReportTimeout)
	defer cancel()
	if _, err := e.reporterClient.Report(reportCtx,
		&reporterv1.ReportRequest{ExecutionState: state}, grpc.WaitForReady(true)); err != nil {
		e.logger.Error("上报最终状态失败", elog.FieldErr(err))
	}
}

func initialState(req *executorv1.ExecuteRequest, nodeID string) *executorv1.ExecutionState {
	return &executorv1.ExecutionState{
		Id: req.GetEid(), TaskId: req.GetTaskId(), TaskName: req.GetTaskName(),
		Status: executorv1.ExecutionStatus_RUNNING, ExecutorNodeId: nodeID,
	}
}

func executionContext(ctx context.Context, fallbackTenantID int64) context.Context {
	// 优先使用拦截器注入的可信租户，兼容内部调用时再回退请求字段。
	tenantID := ctxutil.GetTenantID(ctx).Int64()
	if tenantID == 0 {
		tenantID = fallbackTenantID
	}
	// 后台执行不继承 RPC deadline；其生命周期由 executions 中保存的 cancel 控制。
	runCtx := context.Background()
	if tenantID > 0 {
		runCtx = tenant.Set(runCtx, tenantID)
	}
	return runCtx
}

func resolveFinalStatus(ctx context.Context, err error) executorv1.ExecutionStatus {
	if errors.Is(context.Cause(ctx), execution.ErrTerminated) {
		return executorv1.ExecutionStatus_CANCELLED
	}
	if ctx.Err() != nil {
		return executorv1.ExecutionStatus_FAILED_RESCHEDULABLE
	}
	if err != nil {
		return executorv1.ExecutionStatus_FAILED
	}
	return executorv1.ExecutionStatus_SUCCESS
}

func logFinalStatus(logger *elog.Component, status executorv1.ExecutionStatus, err error) {
	switch status {
	case executorv1.ExecutionStatus_FAILED_RESCHEDULABLE:
		logger.Warn("任务被中断")
	case executorv1.ExecutionStatus_CANCELLED:
		logger.Warn("任务被强制终止")
	case executorv1.ExecutionStatus_FAILED:
		logger.Error("任务执行失败", elog.FieldErr(err))
	case executorv1.ExecutionStatus_SUCCESS:
		logger.Info("任务执行成功")
	}
}
