package preview

import (
	"context"
	"fmt"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/service/dispatcher"
	"github.com/Duke1616/etask/internal/service/invoker"
	programSvc "github.com/Duke1616/etask/internal/service/program"
	runnerSvc "github.com/Duke1616/etask/internal/service/runner"
	taskSvc "github.com/Duke1616/etask/internal/service/task"
	"github.com/gotomicro/ego/core/elog"
)

const (
	defaultTimeoutSeconds int64 = 300
	maxTimeoutSeconds     int64 = 3600
)

// RunCommand 描述一次程序试运行使用的执行单元和临时参数。
type RunCommand struct {
	RunnerID            int64
	Params              map[string]string
	Variables           []domain.RunnerVariable
	MaxExecutionSeconds int64
}

// Service 定义程序试运行能力。
type Service interface {
	// Run 创建试运行记录，并异步派发到正式执行器链路。
	Run(ctx context.Context, command RunCommand) (domain.TaskExecution, error)
	// Status 查询当前租户下的试运行状态。
	Status(ctx context.Context, executionID int64) (domain.TaskExecution, error)
	// Logs 查询当前租户下的试运行日志。
	Logs(ctx context.Context, executionID, minID int64, limit int) ([]domain.TaskExecutionLog, int64, error)
}

type service struct {
	programSvc programSvc.Service
	runnerSvc  runnerSvc.Service
	execSvc    taskSvc.ExecutionService
	logSvc     taskSvc.LogService
	invoker    invoker.Invoker
	routes     dispatcher.RoutePlanner
	logger     *elog.Component
}

// NewService 创建程序试运行服务。
func NewService(
	programService programSvc.Service,
	runnerService runnerSvc.Service,
	executionService taskSvc.ExecutionService,
	logService taskSvc.LogService,
	executionInvoker invoker.Invoker,
	routePlanner dispatcher.RoutePlanner,
) Service {
	return &service{
		programSvc: programService,
		runnerSvc:  runnerService,
		execSvc:    executionService,
		logSvc:     logService,
		invoker:    executionInvoker,
		routes:     routePlanner,
		logger:     elog.DefaultLogger.With(elog.FieldComponentName("service.codebook.preview")),
	}
}

func (s *service) Run(ctx context.Context, command RunCommand) (domain.TaskExecution, error) {
	// 校验参数、解析 Runner、组装变量与依赖程序快照
	prepared, err := s.prepare(ctx, command)
	if err != nil {
		return domain.TaskExecution{}, err
	}

	// 构建执行草稿并由路由规划器匹配目标资源池与节点
	draft := s.buildDraft(prepared)
	route, err := s.routes.Plan(ctx, draft.Task)
	if err != nil {
		return domain.TaskExecution{}, fmt.Errorf("规划试运行路由失败: %w", err)
	}
	if route.Execution.Transport != prepared.runner.Kind.Transport() {
		return domain.TaskExecution{}, fmt.Errorf("执行单元类型 %s 与资源池传输通道 %s 不一致",
			prepared.runner.Kind, route.Execution.Transport)
	}

	draft.Task = route.Task
	draft.Route = route.Execution
	if draft.Task.ExecMode.IsPull() {
		draft.Status = domain.TaskExecutionStatusWaitingPull
	}

	// 持久化试运行执行快照记录
	execution, err := s.execSvc.CreatePreview(ctx, draft, prepared.sourceProjectID)
	if err != nil {
		return domain.TaskExecution{}, fmt.Errorf("创建试运行失败: %w", err)
	}

	// PUSH 模式下异步触发执行器调用；PULL 模式等待 Agent 主动拉取
	if !execution.Task.ExecMode.IsPull() {
		runCtx := route.Context(context.WithoutCancel(ctx))
		go s.invoke(runCtx, execution)
	}
	return execution, nil
}

func (s *service) Status(ctx context.Context, executionID int64) (domain.TaskExecution, error) {
	if executionID <= 0 {
		return domain.TaskExecution{}, fmt.Errorf("试运行记录 ID 非法: %d", executionID)
	}
	execution, err := s.execSvc.FindByID(ctx, executionID)
	if err != nil {
		return domain.TaskExecution{}, err
	}
	if !execution.Source.IsCodebookPreview() {
		return domain.TaskExecution{}, fmt.Errorf("执行记录 %d 不是 Codebook 试运行", executionID)
	}
	return execution, nil
}

func (s *service) Logs(ctx context.Context, executionID, minID int64, limit int) ([]domain.TaskExecutionLog, int64, error) {
	if _, err := s.Status(ctx, executionID); err != nil {
		return nil, 0, err
	}
	if minID < 0 || limit <= 0 || limit > 1000 {
		return nil, 0, fmt.Errorf("试运行日志分页参数非法: min_id=%d limit=%d", minID, limit)
	}
	return s.logSvc.GetLogs(ctx, executionID, minID, limit)
}

func (s *service) invoke(ctx context.Context, execution domain.TaskExecution) {
	// 异步协程 Panic 兜底防护，防止执行异常击垮主服务进程
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("试运行异步执行异常 panic",
				elog.Int64("executionID", execution.ID),
				elog.Any("recover", r))
			_ = s.execSvc.UpdateState(ctx, domain.ExecutionState{
				ID:         execution.ID,
				TaskID:     execution.Task.ID,
				TaskName:   execution.Task.Name,
				Status:     domain.TaskExecutionStatusFailed,
				TaskResult: fmt.Sprintf("执行节点发生未知崩溃: %v", r),
			})
		}
	}()

	state, err := s.invoker.Run(ctx, execution)
	if err != nil {
		state = domain.ExecutionState{
			ID:         execution.ID,
			TaskID:     execution.Task.ID,
			TaskName:   execution.Task.Name,
			Status:     domain.TaskExecutionStatusFailed,
			TaskResult: fmt.Sprintf("调用执行节点失败: %v", err),
		}
	}
	if updateErr := s.execSvc.UpdateState(ctx, state); updateErr != nil {
		s.logger.Error("更新 Codebook 试运行状态失败",
			elog.Int64("executionID", execution.ID), elog.FieldErr(updateErr))
	}
}
