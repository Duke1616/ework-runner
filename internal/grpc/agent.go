package grpc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Duke1616/eiam/pkg/ctxutil"
	executorv1 "github.com/Duke1616/etask/api/proto/gen/etask/executor/v1"
	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/errs"
	programmapper "github.com/Duke1616/etask/internal/execution/program"
	"github.com/Duke1616/etask/internal/repository"
	poolSvc "github.com/Duke1616/etask/internal/service/pool"
	"github.com/Duke1616/etask/internal/service/task"
	"github.com/gotomicro/ego/core/elog"
	"github.com/samber/lo"
)

// AgentServer 实现调度中心的 Agent 拉取服务
type AgentServer struct {
	executorv1.UnimplementedAgentServiceServer
	executorv1.UnimplementedTaskExecutionServiceServer
	execRepo   repository.TaskExecutionRepository
	execSvc    task.ExecutionService
	logSvc     task.LogService
	authorizer poolSvc.ExecutionPoolAuthorizer
	logger     *elog.Component
}

func NewAgentServer(
	execRepo repository.TaskExecutionRepository,
	execSvc task.ExecutionService,
	logSvc task.LogService,
	authorizer poolSvc.ExecutionPoolAuthorizer,
) *AgentServer {
	return &AgentServer{
		execRepo:   execRepo,
		execSvc:    execSvc,
		logSvc:     logSvc,
		authorizer: authorizer,
		logger:     elog.DefaultLogger.With(elog.FieldComponentName("grpc.AgentServer")),
	}
}

// PullTask 响应执行节点的拉取请求
func (s *AgentServer) PullTask(ctx context.Context, req *executorv1.PullTaskRequest) (*executorv1.PullTaskResponse, error) {
	serviceName, nodeID, handlerNames, err := validatePullTaskRequest(req)
	if err != nil {
		return nil, err
	}

	// 1. 设置最大长轮询时间
	timeoutCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		// 在数据库中寻找并乐观抢占一条状态为 WAITING_PULL 且 Service(Group) 匹配的执行记录
		// 这里将 nodeID 真实落库记录为 executor_node_id。
		exec, err := s.execRepo.ClaimPullTask(timeoutCtx, serviceName, nodeID, handlerNames)
		if err == nil {
			response, deliver, prepareErr := s.preparePulledTask(timeoutCtx, exec, nodeID)
			if prepareErr != nil {
				return nil, prepareErr
			}
			if deliver {
				return response, nil
			}
			continue
		}
		if !errors.Is(err, errs.ErrExecutionNotFound) && !errors.Is(err, errs.ErrExecutionClaimConflict) {
			return nil, fmt.Errorf("拉取待执行任务失败: %w", err)
		}

		select {
		case <-timeoutCtx.Done():
			// 憋了 25 秒还是没有活，正常返回让客户端进入下一轮拉取
			return &executorv1.PullTaskResponse{HasTask: false}, nil
		case <-ticker.C:
			// 没有活儿，稍微等 2 秒继续尝试抢占
			continue
		}
	}
}

func validatePullTaskRequest(req *executorv1.PullTaskRequest) (string, string, []string, error) {
	serviceName := strings.TrimSpace(req.GetServiceName())
	if serviceName == "" {
		return "", "", nil, fmt.Errorf("执行服务名称不能为空")
	}
	nodeID := strings.TrimSpace(req.GetNodeId())
	if nodeID == "" {
		return "", "", nil, fmt.Errorf("执行节点 ID 不能为空")
	}
	handlerNames := normalizeHandlerNames(req.GetHandlers())
	if len(handlerNames) == 0 {
		return "", "", nil, fmt.Errorf("执行节点至少需要声明一个处理器")
	}
	return serviceName, nodeID, handlerNames, nil
}

// preparePulledTask 将已抢占的执行记录转换为 Agent 指令。
// 返回 deliver=false 表示该任务已被标记为无效或未授权，调用方应继续轮询。
func (s *AgentServer) preparePulledTask(
	ctx context.Context,
	exec domain.TaskExecution,
	nodeID string,
) (*executorv1.PullTaskResponse, bool, error) {
	artifacts, err := domain.ArtifactRefsToProto(exec.Artifacts)
	if err != nil {
		s.finishClaimedExecution(ctx, exec, nodeID, domain.TaskExecutionStatusFailed, "执行制品引用非法: "+err.Error())
		return nil, false, err
	}
	program, err := programmapper.ToProto(exec.Program)
	if err != nil {
		s.finishClaimedExecution(ctx, exec, nodeID, domain.TaskExecutionStatusFailed, "执行程序来源非法: "+err.Error())
		return nil, false, err
	}
	allowed, err := s.isExecutionAllowed(ctx, exec)
	if err != nil {
		s.finishClaimedExecution(ctx, exec, nodeID, domain.TaskExecutionStatusFailedRescheduled, "校验执行资源池授权失败: "+err.Error())
		return nil, false, err
	}
	if !allowed {
		s.finishClaimedExecution(ctx, exec, nodeID, domain.TaskExecutionStatusFailed, "执行资源池授权已被撤销")
		return nil, false, nil
	}

	handlerName := ""
	if exec.Task.GrpcConfig != nil {
		handlerName = exec.Task.GrpcConfig.HandlerName
	}
	return &executorv1.PullTaskResponse{
		HasTask: true,
		TaskReq: &executorv1.ExecuteRequest{
			Eid:             exec.ID,
			TaskId:          exec.Task.ID,
			TaskName:        exec.Task.Name,
			TaskHandlerName: handlerName,
			Params:          exec.GRPCParams(),
			// PULL 请求是 Agent 在后台发起的，gRPC Metadata 不携带任务租户，
			// 因此必须把租户 ID 放入指令消息，供 Agent 重建执行上下文。
			TenantId:    exec.TenantID,
			Artifacts:   artifacts,
			Program:     program,
			VariableSet: exec.Variables.ToProto(),
		},
	}, true, nil
}

func normalizeHandlerNames(values []string) []string {
	return lo.Uniq(lo.FilterMap(values, func(value string, _ int) (string, bool) {
		value = strings.TrimSpace(value)
		return value, value != ""
	}))
}

func (s *AgentServer) isExecutionAllowed(ctx context.Context, exec domain.TaskExecution) (bool, error) {
	if exec.Task.GrpcConfig == nil {
		return true, nil
	}
	ctx = ctxutil.WithTenantID(ctx, exec.TenantID)
	return s.authorizer.IsAllowed(ctx, poolSvc.CheckBindingRequest{
		PoolName:    exec.Task.GrpcConfig.ServiceName,
		HandlerName: exec.Task.GrpcConfig.HandlerName,
	})
}

func (s *AgentServer) finishClaimedExecution(ctx context.Context, exec domain.TaskExecution,
	nodeID string, status domain.TaskExecutionStatus, result string) {
	updateCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	err := s.execSvc.UpdateState(updateCtx, domain.ExecutionState{
		ID:             exec.ID,
		TaskID:         exec.Task.ID,
		TaskName:       exec.Task.Name,
		Status:         status,
		ExecutorNodeID: nodeID,
		TaskResult:     result,
	})
	if err != nil {
		s.logger.Error("结束已领取的 PULL 任务失败",
			elog.Int64("taskID", exec.Task.ID),
			elog.Int64("executionID", exec.ID),
			elog.FieldErr(err))
	}
}

// ListTaskExecutions 列出任务执行记录
func (s *AgentServer) ListTaskExecutions(ctx context.Context, req *executorv1.ListTaskExecutionsRequest) (*executorv1.ListTaskExecutionsResponse, error) {
	executions, err := s.execRepo.FindByTaskID(ctx, req.GetTaskId())
	if err != nil {
		s.logger.Error("获取执行记录失败", elog.Int64("taskID", req.GetTaskId()), elog.FieldErr(err))
		return nil, err
	}

	return &executorv1.ListTaskExecutionsResponse{
		Executions: lo.Map(executions, func(e domain.TaskExecution, _ int) *executorv1.TaskExecution {
			return toProtoTaskExecution(e)
		}),
	}, nil
}

// GetTaskExecution 根据执行 ID 获取执行记录。
func (s *AgentServer) GetTaskExecution(ctx context.Context,
	req *executorv1.GetTaskExecutionRequest) (*executorv1.GetTaskExecutionResponse, error) {
	if req.GetExecutionId() <= 0 {
		return nil, fmt.Errorf("执行 ID 非法: %d", req.GetExecutionId())
	}
	execution, err := s.execSvc.FindByID(ctx, req.GetExecutionId())
	if err != nil {
		return nil, err
	}
	return &executorv1.GetTaskExecutionResponse{Execution: toProtoTaskExecution(execution)}, nil
}

// GetExecutionLogs 获取执行日志
func (s *AgentServer) GetExecutionLogs(ctx context.Context, req *executorv1.GetExecutionLogsRequest) (*executorv1.GetExecutionLogsResponse, error) {
	logs, _, err := s.logSvc.GetLogs(ctx, req.GetExecutionId(), req.GetMinId(), int(req.GetLimit()))
	if err != nil {
		s.logger.Error("获取日志失败", elog.Int64("executionID", req.GetExecutionId()), elog.FieldErr(err))
		return nil, err
	}

	pbLogs := lo.Map(logs, func(l domain.TaskExecutionLog, _ int) *executorv1.ExecutionLog {
		return &executorv1.ExecutionLog{
			Id:      l.ID,
			Time:    l.CTime,
			Content: l.Content,
		}
	})
	maxID := lo.Reduce(logs, func(maxID int64, l domain.TaskExecutionLog, _ int) int64 {
		if l.ID > maxID {
			return l.ID
		}
		return maxID
	}, int64(0))

	return &executorv1.GetExecutionLogsResponse{
		Logs:  pbLogs,
		MaxId: maxID,
	}, nil
}

// BatchListTaskExecutions 批量列出任务执行记录
func (s *AgentServer) BatchListTaskExecutions(ctx context.Context, req *executorv1.BatchListTaskExecutionsRequest) (*executorv1.BatchListTaskExecutionsResponse, error) {
	taskIDs := req.GetTaskIds()

	// 过滤掉无效的 task_id (如 0 或负数)，防止数据库产生无意义的扫描
	validTaskIDs := lo.Filter(taskIDs, func(id int64, _ int) bool { return id > 0 })

	if len(validTaskIDs) == 0 {
		return &executorv1.BatchListTaskExecutionsResponse{
			Results: make(map[int64]*executorv1.TaskExecutionList),
		}, nil
	}

	executions, err := s.execRepo.FindByTaskIDs(ctx, validTaskIDs)
	if err != nil {
		s.logger.Error("批量获取执行记录失败", elog.Any("taskIDs", taskIDs), elog.FieldErr(err))
		return nil, err
	}

	grouped := lo.GroupBy(executions, func(e domain.TaskExecution) int64 {
		return e.Task.ID
	})
	results := make(map[int64]*executorv1.TaskExecutionList, len(grouped))
	for taskID, taskExecutions := range grouped {
		results[taskID] = &executorv1.TaskExecutionList{
			Executions: lo.Map(taskExecutions, func(e domain.TaskExecution, _ int) *executorv1.TaskExecution {
				return toProtoTaskExecution(e)
			}),
		}
	}

	return &executorv1.BatchListTaskExecutionsResponse{
		Results: results,
	}, nil
}

func toProtoTaskExecution(e domain.TaskExecution) *executorv1.TaskExecution {
	return &executorv1.TaskExecution{
		Id:              e.ID,
		TaskId:          e.Task.ID,
		TaskName:        e.Task.Name,
		StartTime:       e.StartTime,
		EndTime:         e.EndTime,
		Status:          executorv1.ExecutionStatus(executorv1.ExecutionStatus_value[e.Status.String()]),
		RunningProgress: e.RunningProgress,
		ExecutorNodeId:  e.ExecutorNodeID,
		TaskResult:      e.TaskResult,
	}
}
