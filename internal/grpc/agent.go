package grpc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

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
	bindingSvc poolSvc.BindingService
	logger     *elog.Component
}

func NewAgentServer(
	execRepo repository.TaskExecutionRepository,
	execSvc task.ExecutionService,
	logSvc task.LogService,
	bindingSvc poolSvc.BindingService,
) *AgentServer {
	return &AgentServer{
		execRepo:   execRepo,
		execSvc:    execSvc,
		logSvc:     logSvc,
		bindingSvc: bindingSvc,
		logger:     elog.DefaultLogger.With(elog.FieldComponentName("grpc.AgentServer")),
	}
}

// PullTask 响应执行节点的拉取请求
func (s *AgentServer) PullTask(ctx context.Context, req *executorv1.PullTaskRequest) (*executorv1.PullTaskResponse, error) {
	serviceName := req.GetServiceName()
	nodeId := req.GetNodeId()
	if serviceName == "" {
		return nil, fmt.Errorf("执行服务名称不能为空")
	}
	if nodeId == "" {
		return nil, fmt.Errorf("执行节点 ID 不能为空")
	}
	handlerNames := normalizeHandlerNames(req.GetHandlers())
	if len(handlerNames) == 0 {
		return nil, fmt.Errorf("执行节点至少需要声明一个处理器")
	}

	// 1. 设置最大长轮询时间
	timeoutCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		// 在数据库中寻找并乐观抢占一条状态为 WAITING_PULL 且 Service(Group) 匹配的执行记录
		// 这里将 nodeId 真实落库记录为 executor_node_id
		exec, err := s.execRepo.ClaimPullTask(timeoutCtx, serviceName, nodeId, handlerNames)
		if err == nil {
			artifacts, artifactErr := domain.ArtifactRefsToProto(exec.Artifacts)
			if artifactErr != nil {
				s.finishClaimedExecution(timeoutCtx, exec, nodeId,
					domain.TaskExecutionStatusFailed, "执行制品引用非法: "+artifactErr.Error())
				return nil, artifactErr
			}
			program, programErr := programmapper.ToProto(exec.Program)
			if programErr != nil {
				s.finishClaimedExecution(timeoutCtx, exec, nodeId,
					domain.TaskExecutionStatusFailed, "执行程序来源非法: "+programErr.Error())
				return nil, programErr
			}
			allowed, authErr := s.isExecutionAllowed(timeoutCtx, exec)
			if authErr != nil {
				s.finishClaimedExecution(timeoutCtx, exec, nodeId,
					domain.TaskExecutionStatusFailedRescheduled, "校验执行资源池授权失败: "+authErr.Error())
				return nil, authErr
			}
			if !allowed {
				s.finishClaimedExecution(timeoutCtx, exec, nodeId,
					domain.TaskExecutionStatusFailed, "执行资源池授权已被撤销")
				continue
			}

			handlerName := ""
			if exec.Task.GrpcConfig != nil {
				handlerName = exec.Task.GrpcConfig.HandlerName
			}
			// 2. 抢占成功，直接将执行指令下发
			return &executorv1.PullTaskResponse{
				HasTask: true,
				TaskReq: &executorv1.ExecuteRequest{
					Eid:             exec.ID,
					TaskId:          exec.Task.ID,
					TaskName:        exec.Task.Name,
					TaskHandlerName: handlerName,
					Params:          exec.GRPCParams(),
					// 在 PULL (拉取) 模式下，因为 Agent 边缘节点的长轮询请求是系统级无租户背景发起的，
					// gRPC 链路请求头中无法携带租户 Metadata。因此必须将任务所属的 TenantID
					// 显式塞入 proto Payload 消息体中下发，供 Agent 侧反向提取并重建租户 context 树。
					TenantId:    exec.TenantID,
					Artifacts:   artifacts,
					Program:     program,
					VariableSet: exec.Variables.ToProto(),
				},
			}, nil
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

func normalizeHandlerNames(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func (s *AgentServer) isExecutionAllowed(ctx context.Context, exec domain.TaskExecution) (bool, error) {
	if exec.Task.GrpcConfig == nil || s.bindingSvc == nil {
		return true, nil
	}
	return s.bindingSvc.IsAllowed(ctx, poolSvc.CheckBindingRequest{
		TenantID:    exec.TenantID,
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

	results := make(map[int64]*executorv1.TaskExecutionList)
	for _, e := range executions {
		pbExec := toProtoTaskExecution(e)

		if list, ok := results[e.Task.ID]; ok {
			list.Executions = append(list.Executions, pbExec)
		} else {
			results[e.Task.ID] = &executorv1.TaskExecutionList{
				Executions: []*executorv1.TaskExecution{pbExec},
			}
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
