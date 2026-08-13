package task

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/errs"
	"github.com/Duke1616/etask/internal/event"
	"github.com/Duke1616/etask/internal/repository"
	"github.com/Duke1616/etask/internal/service/acquirer"
	artifactSvc "github.com/Duke1616/etask/internal/service/artifact"
	programSvc "github.com/Duke1616/etask/internal/service/program"
	runnerSvc "github.com/Duke1616/etask/internal/service/runner"
	taskbinding "github.com/Duke1616/etask/internal/service/task/binding"
	"github.com/Duke1616/etask/internal/sse"
	"github.com/Duke1616/etask/pkg/grpc/registry"
	"github.com/Duke1616/etask/pkg/retry"
	"github.com/gotomicro/ego/core/elog"
)

//go:generate go tool mockgen -source=./execution_service.go -package=taskmocks -destination=./mocks/execution_service.mock.go -typed

// ExecutionService 任务执行服务接口
type ExecutionService interface {
	// Create 创建任务执行实例
	Create(ctx context.Context, execution domain.TaskExecution) (domain.TaskExecution, error)
	// CreatePreview 创建不依赖正式任务的 Codebook 试运行执行实例。
	CreatePreview(ctx context.Context, execution domain.TaskExecution, sourceProjectID int64) (domain.TaskExecution, error)
	// CreateWorkflow 幂等创建由外部工作流提交的正式执行实例。
	CreateWorkflow(ctx context.Context, execution domain.TaskExecution,
		sourceProjectID int64) (domain.TaskExecution, bool, error)
	// FindByID 根据ID获取执行实例
	FindByID(ctx context.Context, id int64) (domain.TaskExecution, error)
	// FindWorkflowByRequestID 根据工作流幂等请求标识查询执行。
	FindWorkflowByRequestID(ctx context.Context, requestID string) (domain.TaskExecution, bool, error)
	// FindRetryableExecutions 查找所有可以重试的执行记录
	// limit: 查询结果数量限制
	FindRetryableExecutions(ctx context.Context, limit int) ([]domain.TaskExecution, error)
	// FindReschedulableExecutions 查找所有可以重调度的执行记录
	FindReschedulableExecutions(ctx context.Context, limit int) ([]domain.TaskExecution, error)
	// FindExecutionByTaskIDAndPlanExecID 根据任务和计划执行 ID 查询执行记录。
	FindExecutionByTaskIDAndPlanExecID(ctx context.Context, taskID int64, planExecID int64) (domain.TaskExecution, error)
	// FindTimeoutExecutions 查找超时的执行记录
	FindTimeoutExecutions(ctx context.Context, limit int) ([]domain.TaskExecution, error)
	// RequeuePull 将失败的 PULL 执行重新放回等待拉取队列。
	RequeuePull(ctx context.Context, executionID int64) error

	// SetRunningState 设置任务为运行状态并更新进度
	SetRunningState(ctx context.Context, id int64, progress int32, executorNodeID string) error
	// UpdateRunningProgress 更新任务执行进度（仅在RUNNING状态下有效）
	UpdateRunningProgress(ctx context.Context, id int64, progress int32, executorNodeID string) error
	// UpdateRetryResult 仅在当前状态符合预期时更新重试结果。
	UpdateRetryResult(ctx context.Context, id, retryCount, nextRetryTime int64,
		expectedStatus, status domain.TaskExecutionStatus, progress int32, endTime int64,
		scheduleParams map[string]string, executorNodeID string) error
	// UpdateScheduleResult 仅在当前状态符合预期时更新调度结果。
	// 返回 false 表示状态已被其他请求推进，当前请求没有写入。
	UpdateScheduleResult(ctx context.Context, id int64, expectedStatuses []domain.TaskExecutionStatus,
		status domain.TaskExecutionStatus, progress int32, endTime int64,
		scheduleParams map[string]string, executorNodeID string, taskResult string) (bool, error)

	// AppendExecutionLogs 保存一批日志并广播给 SSE 订阅者。
	AppendExecutionLogs(ctx context.Context, executionID, taskID int64, logs []string) error
	// UpdateState 更新执行节点上报的执行状态
	UpdateState(ctx context.Context, state domain.ExecutionState) error
	// ListByTaskID 分页查找执行记录
	ListByTaskID(ctx context.Context, taskID int64, offset, limit int) ([]domain.TaskExecution, int64, error)
}

// RequeuePull 将执行记录恢复为等待拉取状态，后续由 Executor 原子抢占。
func (s *executionService) RequeuePull(ctx context.Context, executionID int64) error {
	if executionID <= 0 {
		return fmt.Errorf("执行 ID 非法")
	}
	return s.repo.UpdateStatus(ctx, executionID, []domain.TaskExecutionStatus{
		domain.TaskExecutionStatusFailedRetryable,
		domain.TaskExecutionStatusFailedRescheduled,
	}, domain.TaskExecutionStatusWaitingPull)
}

type executionService struct {
	nodeID       string
	repo         repository.TaskExecutionRepository
	taskSvc      Service
	logSvc       LogService             // 日志服务
	taskAcquirer acquirer.TaskAcquirer  // 任务抢占器
	producer     event.CompleteProducer // 任务完成事件生产者
	registry     registry.Registry
	resolvers    *taskbinding.Registry
	artifactSvc  artifactSvc.Service
	programSvc   programSvc.Service
	runnerSvc    runnerSvc.Service
	events       *sse.Hubs
	logger       *elog.Component
}

// NewExecutionService 创建任务执行服务实例
func NewExecutionService(
	nodeID string,
	repo repository.TaskExecutionRepository,
	taskSvc Service,
	logSvc LogService,
	producer event.CompleteProducer,
	registry registry.Registry,
	resolvers *taskbinding.Registry,
	artifactSvc artifactSvc.Service,
	programSvc programSvc.Service,
	runnerService runnerSvc.Service,
	events *sse.Hubs,
) ExecutionService {
	return &executionService{
		nodeID:      nodeID,
		repo:        repo,
		taskSvc:     taskSvc,
		logSvc:      logSvc,
		producer:    producer,
		registry:    registry,
		resolvers:   resolvers,
		artifactSvc: artifactSvc,
		programSvc:  programSvc,
		runnerSvc:   runnerService,
		events:      events,
		logger:      elog.DefaultLogger.With(elog.FieldComponentName("service.execution")),
	}
}

func (s *executionService) Create(ctx context.Context, execution domain.TaskExecution) (domain.TaskExecution, error) {
	execution.Source = domain.TaskExecutionSourceTask
	if err := execution.Route.Validate(); err != nil {
		return domain.TaskExecution{}, fmt.Errorf("执行路由非法: %w", err)
	}
	// 执行记录保存完整任务快照，后续编辑任务不会改变本次运行语义。
	snapshot, selection, variables, err := s.buildTaskSnapshot(ctx, execution.Task)
	if err != nil {
		return domain.TaskExecution{}, err
	}
	// 路由中的派发模式属于本次执行快照，不能被任务表里的上一次模式覆盖。
	snapshot.ExecMode = execution.Route.DispatchMode
	execution.Task = snapshot
	execution.Variables = variables
	execution.Program = selection.Program
	// 执行记录同时固定项目源码和依赖制品，运行时不会漂移到新版本。
	if err = s.resolveArtifacts(ctx, &execution, selection.SourceProjectID); err != nil {
		return domain.TaskExecution{}, err
	}
	if execution.TenantID == 0 {
		execution.TenantID = snapshot.TenantID
	}

	created, err := s.repo.Create(ctx, execution)
	if err != nil {
		return created, err
	}

	s.broadcastExecutionEvent(created.ID)
	return created, nil
}

func (s *executionService) CreatePreview(ctx context.Context, execution domain.TaskExecution,
	sourceProjectID int64) (domain.TaskExecution, error) {
	execution.Source = domain.TaskExecutionSourceCodebookPreview
	if err := s.prepareDetachedExecution(ctx, &execution, sourceProjectID); err != nil {
		return domain.TaskExecution{}, err
	}
	created, err := s.repo.Create(ctx, execution)
	if err != nil {
		return domain.TaskExecution{}, err
	}
	s.broadcastExecutionEvent(created.ID)
	return created, nil
}

func (s *executionService) CreateWorkflow(ctx context.Context, execution domain.TaskExecution,
	sourceProjectID int64) (domain.TaskExecution, bool, error) {
	if strings.TrimSpace(execution.RequestID) == "" {
		return domain.TaskExecution{}, false, fmt.Errorf("工作流执行缺少幂等请求标识")
	}
	if existing, ok, err := s.repo.FindByRequestID(ctx, domain.TaskExecutionSourceWorkflow,
		execution.RequestID); err != nil {
		return domain.TaskExecution{}, false, err
	} else if ok {
		return existing, false, nil
	}

	execution.Source = domain.TaskExecutionSourceWorkflow
	if err := s.prepareDetachedExecution(ctx, &execution, sourceProjectID); err != nil {
		return domain.TaskExecution{}, false, err
	}
	created, err := s.repo.Create(ctx, execution)
	if err != nil {
		// 并发提交可能同时通过首次查询；唯一约束的获胜记录就是幂等结果。
		if existing, ok, findErr := s.repo.FindByRequestID(ctx, domain.TaskExecutionSourceWorkflow,
			execution.RequestID); findErr == nil && ok {
			return existing, false, nil
		}
		return domain.TaskExecution{}, false, err
	}
	s.broadcastExecutionEvent(created.ID)
	return created, true, nil
}

// prepareDetachedExecution 为不绑定 etask 正式任务的执行补齐租户、路由和依赖来源。
func (s *executionService) prepareDetachedExecution(ctx context.Context, execution *domain.TaskExecution,
	sourceProjectID int64) error {
	execution.Task.ID = 0
	execution.Task.Type = domain.TaskTypeOneTime
	if err := execution.Route.Validate(); err != nil {
		return fmt.Errorf("执行路由非法: %w", err)
	}
	execution.Task.ExecMode = execution.Route.DispatchMode
	execution.Task.TenantID = ctxutil.GetTenantID(ctx).Int64()
	if execution.Task.TenantID <= 0 {
		return fmt.Errorf("缺少租户上下文，无法创建执行记录")
	}
	if err := s.taskSvc.AuthorizeExecutionPool(ctx, execution.Task); err != nil {
		return err
	}
	if execution.Program != nil {
		if err := execution.Program.Validate(); err != nil {
			return fmt.Errorf("程序来源非法: %w", err)
		}
	}
	if err := s.resolveArtifacts(ctx, execution, sourceProjectID); err != nil {
		return err
	}
	execution.TenantID = execution.Task.TenantID
	return nil
}

func (s *executionService) resolveArtifacts(ctx context.Context, execution *domain.TaskExecution,
	sourceProjectID int64) error {
	if execution.Program == nil {
		return nil
	}
	artifacts, err := s.artifactSvc.ResolveExecution(ctx, sourceProjectID)
	if err != nil {
		return err
	}
	execution.Artifacts = artifacts
	return nil
}

func (s *executionService) buildTaskSnapshot(ctx context.Context,
	task domain.Task) (domain.Task, programSvc.Resolution, *domain.ExecutionVariableSet, error) {
	// 重新读取持久化任务，调度列表中的旧对象只提供本次动态调度参数。
	snapshot, err := s.taskSvc.GetByID(ctx, task.ID)
	if err != nil {
		return domain.Task{}, programSvc.Resolution{}, nil, fmt.Errorf("获取Task信息失败: %w", err)
	}

	snapshot.UpdateScheduleParams(task.ScheduleParams)
	variables, err := s.applyRunnerDefaults(ctx, &snapshot)
	if err != nil {
		return domain.Task{}, programSvc.Resolution{}, nil, err
	}
	if err = s.taskSvc.AuthorizeExecutionPool(ctx, snapshot); err != nil {
		return domain.Task{}, programSvc.Resolution{}, nil, err
	}
	selection, err := s.programSvc.Resolve(ctx, snapshot.Program)
	if err != nil {
		return domain.Task{}, programSvc.Resolution{}, nil, err
	}

	// Runner 任务的默认参数和变量已经由执行单元快照解析完成，不再运行旧参数绑定。
	if snapshot.GrpcConfig == nil || snapshot.RunnerID != 0 {
		applyPendingParamOverrides(&snapshot)
		return snapshot, selection, variables, nil
	}

	// Codebook 等绑定在执行创建阶段解析，并写入私有参数副本。
	resolved, err := s.resolvers.Resolve(ctx, snapshot.GrpcConfig.HandlerName,
		snapshot.GrpcConfig.Params, snapshot.Metadata)
	if err != nil {
		return domain.Task{}, programSvc.Resolution{}, nil, err
	}
	if len(resolved) == 0 {
		applyPendingParamOverrides(&snapshot)
		return snapshot, selection, variables, nil
	}

	resolvedParams := make(map[string]string, len(snapshot.GrpcConfig.Params)+len(resolved))
	for k, v := range snapshot.GrpcConfig.Params {
		resolvedParams[k] = v
	}
	for k, v := range resolved {
		resolvedParams[k] = v
	}
	snapshot.GrpcConfig.Params = resolvedParams
	applyPendingParamOverrides(&snapshot)
	return snapshot, selection, variables, nil
}

func applyPendingParamOverrides(task *domain.Task) {
	if task.GrpcConfig == nil || len(task.PendingParamOverrides) == 0 {
		return
	}
	config := *task.GrpcConfig
	params := make(map[string]string, len(config.Params)+len(task.PendingParamOverrides))
	for key, value := range config.Params {
		params[key] = value
	}
	for key, value := range task.PendingParamOverrides {
		params[key] = value
	}
	config.Params = params
	task.GrpcConfig = &config
}

// applyRunnerDefaults 在创建执行快照时读取最新默认参数，任务参数覆盖同名默认值。
func (s *executionService) applyRunnerDefaults(ctx context.Context,
	task *domain.Task) (*domain.ExecutionVariableSet, error) {
	if task.RunnerID == 0 {
		return nil, nil
	}
	if s.runnerSvc == nil {
		return nil, fmt.Errorf("执行单元服务不可用")
	}
	if task.GrpcConfig == nil {
		return nil, fmt.Errorf("引用执行单元的任务缺少 gRPC 配置")
	}
	spec, err := s.runnerSvc.FindForExecution(ctx, task.RunnerID)
	if err != nil {
		return nil, fmt.Errorf("查询执行单元失败: %w", err)
	}
	runner := spec.Runner
	if runner.Action != domain.RunnerActionRegistered {
		return nil, fmt.Errorf("执行单元未启用")
	}
	params, err := runnerSvc.MergeParameterDefaults(runner.ParameterDefaults, task.GrpcConfig.Params)
	if err != nil {
		return nil, err
	}
	config := *task.GrpcConfig
	config.Params = params
	task.GrpcConfig = &config
	return &domain.ExecutionVariableSet{Items: spec.Variables}, nil
}

func (s *executionService) FindByID(ctx context.Context, id int64) (domain.TaskExecution, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *executionService) FindWorkflowByRequestID(ctx context.Context,
	requestID string) (domain.TaskExecution, bool, error) {
	return s.repo.FindByRequestID(ctx, domain.TaskExecutionSourceWorkflow, requestID)
}

func (s *executionService) FindRetryableExecutions(ctx context.Context, limit int) ([]domain.TaskExecution, error) {
	return s.repo.FindRetryableExecutions(ctx, limit)
}

func (s *executionService) FindReschedulableExecutions(ctx context.Context, limit int) ([]domain.TaskExecution, error) {
	return s.repo.FindReschedulableExecutions(ctx, limit)
}

func (s *executionService) FindExecutionByTaskIDAndPlanExecID(ctx context.Context, taskID, planExecID int64) (domain.TaskExecution, error) {
	return s.repo.FindExecutionByTaskIDAndPlanExecID(ctx, taskID, planExecID)
}

func (s *executionService) FindTimeoutExecutions(ctx context.Context, limit int) ([]domain.TaskExecution, error) {
	return s.repo.FindTimeoutExecutions(ctx, limit)
}

func (s *executionService) SetRunningState(ctx context.Context, id int64, progress int32, executorNodeID string) error {
	err := s.repo.SetRunningState(ctx, id, progress, executorNodeID)
	if err != nil {
		return err
	}
	s.broadcastExecutionEvent(id)
	return nil
}

func (s *executionService) UpdateRunningProgress(ctx context.Context, id int64, progress int32, executorNodeID string) error {
	err := s.repo.UpdateRunningProgress(ctx, id, progress, executorNodeID)
	if err != nil {
		return err
	}
	s.broadcastExecutionEvent(id)
	return nil
}

func (s *executionService) UpdateRetryResult(ctx context.Context, id, retryCount, nextRetryTime int64,
	expectedStatus, status domain.TaskExecutionStatus, progress int32, endTime int64,
	scheduleParams map[string]string, executorNodeID string) error {
	err := s.repo.UpdateRetryResult(ctx, id, retryCount, nextRetryTime, expectedStatus,
		status, progress, endTime, scheduleParams, executorNodeID)
	if err != nil {
		return err
	}
	s.broadcastExecutionEvent(id)
	return nil
}

func (s *executionService) UpdateScheduleResult(ctx context.Context, id int64,
	expectedStatuses []domain.TaskExecutionStatus, status domain.TaskExecutionStatus,
	progress int32, endTime int64, scheduleParams map[string]string,
	executorNodeID string, taskResult string) (bool, error) {
	updated, err := s.repo.UpdateScheduleResult(ctx, id, expectedStatuses, status, progress,
		endTime, scheduleParams, executorNodeID, taskResult)
	if err != nil {
		return false, err
	}
	if !updated {
		// CAS 未命中时确认记录仍然存在；终态冲突可安全忽略，缺失记录仍需上抛。
		if _, findErr := s.repo.GetByID(ctx, id); findErr != nil {
			return false, findErr
		}
		return false, nil
	}
	s.broadcastExecutionEvent(id)
	return true, nil
}

func (s *executionService) AppendExecutionLogs(ctx context.Context,
	executionID, taskID int64, logs []string) error {
	if len(logs) == 0 {
		return nil
	}
	if executionID <= 0 {
		return fmt.Errorf("执行 ID 非法: %d", executionID)
	}
	persisted, err := s.logSvc.AddLog(ctx, domain.TaskExecutionLog{
		ExecutionID: executionID,
		TaskID:      taskID,
		Content:     strings.Join(logs, "\n"),
		CTime:       time.Now().UnixMilli(),
	})
	if err != nil {
		return fmt.Errorf("保存任务日志失败: executionID=%d: %w", executionID, err)
	}
	if s.events != nil && s.events.Logs != nil {
		s.events.Logs.Broadcast(persisted.ExecutionID, sse.TaskLogEvent{
			ID: persisted.ID, TaskID: persisted.TaskID, ExecutionID: persisted.ExecutionID,
			Content: persisted.Content, CTime: persisted.CTime,
		})
	}
	return nil
}

func (s *executionService) UpdateState(ctx context.Context, state domain.ExecutionState) error {
	execution, err := s.FindByID(ctx, state.ID)
	if err != nil {
		return errs.ErrExecutionNotFound
	}

	// 终态获胜：重复终态和晚到的其他回调都幂等忽略，避免消息持续重试。
	if execution.Status.IsTerminalStatus() {
		return nil
	}
	if execution.Source.IsCodebookPreview() {
		return s.updatePreviewState(ctx, execution, state)
	}
	if execution.Source.IsWorkflow() {
		return s.updateWorkflowState(ctx, execution, state)
	}

	switch {
	case state.Status.IsRunning():
		if execution.Status.IsRunning() {
			// 仅更新进度
			return s.updateRunningProgress(ctx, state)
		}
		// 设置为RUNNING状态的同时设置开始时间
		return s.setRunningState(ctx, state)
	case state.Status.IsFailedRetryable():
		err = s.updateRetryState(ctx, execution, state)
		if err != nil {
			// 达到最大重试次数
			if errors.Is(err, errs.ErrExecutionMaxRetriesExceeded) {
				// NOTE: 只发送完成事件,由消费者统一更新终止状态
				return s.sendCompletedEvent(ctx, state, execution)
			}
			// 其他错误才记录并返回
			s.logger.Error("更新任务执行记录的重试结果失败",
				elog.Int64("taskID", state.TaskID),
				elog.String("taskName", state.TaskName),
				elog.Any("state", state),
				elog.FieldErr(err))
			return err
		}
		return nil
	case state.Status.IsFailedRescheduled():
		if state.RequestReschedule {
			// 更新调度信息
			execution.MergeTaskScheduleParams(state.RescheduleParams)
		}
		err = s.updateState(ctx, execution, state)
		if err != nil {
			return fmt.Errorf("更新任务执行记录的重调度结果失败：%w", err)
		}
		return nil
	case state.Status.IsTerminalStatus():
		// 只发送完成事件,由消费者统一更新终止状态,避免重复更新
		return s.sendCompletedEvent(ctx, state, execution)
	default:
		s.logger.Error("非法上报状态",
			elog.Int64("taskID", execution.Task.ID),
			elog.String("taskName", execution.Task.Name),
			elog.String("currentStatus", execution.Status.String()),
			elog.String("targetStatus", state.Status.String()))
		return errs.ErrInvalidTaskExecutionStatus
	}
}

func (s *executionService) updatePreviewState(ctx context.Context, execution domain.TaskExecution, state domain.ExecutionState) error {
	switch {
	case state.Status.IsRunning():
		if execution.Status.IsRunning() {
			return s.updateRunningProgress(ctx, state)
		}
		return s.setRunningState(ctx, state)
	case state.Status.IsSuccess(), state.Status.IsFailed(), state.Status.IsCancelled():
		return s.updateState(ctx, execution, state)
	case state.Status.IsFailedRetryable(), state.Status.IsFailedRescheduled():
		state.Status = domain.TaskExecutionStatusFailed
		return s.updateState(ctx, execution, state)
	default:
		return errs.ErrInvalidTaskExecutionStatus
	}
}

// updateWorkflowState 将 etask 内部可重试状态收敛为一次工作流尝试的明确终态。
// 工作流层需要重试时会创建新的 attempt，而不是复用当前 execution。
func (s *executionService) updateWorkflowState(ctx context.Context, execution domain.TaskExecution,
	state domain.ExecutionState) error {
	switch {
	case state.Status.IsRunning():
		if execution.Status.IsRunning() {
			return s.updateRunningProgress(ctx, state)
		}
		return s.setRunningState(ctx, state)
	case state.Status.IsSuccess(), state.Status.IsFailed(), state.Status.IsCancelled():
		return s.sendCompletedEvent(ctx, state, execution)
	case state.Status.IsFailedRetryable(), state.Status.IsFailedRescheduled():
		state.Status = domain.TaskExecutionStatusFailed
		return s.sendCompletedEvent(ctx, state, execution)
	default:
		return errs.ErrInvalidTaskExecutionStatus
	}
}

func (s *executionService) updateRunningProgress(ctx context.Context, state domain.ExecutionState) error {
	err := s.UpdateRunningProgress(ctx, state.ID, state.RunningProgress, state.ExecutorNodeID)
	if err != nil {
		s.logger.Error("更新运行进度失败",
			elog.Int64("taskID", state.TaskID),
			elog.String("taskName", state.TaskName),
			elog.Any("state", state),
			elog.FieldErr(err))
		return err
	}
	return nil
}

func (s *executionService) setRunningState(ctx context.Context, state domain.ExecutionState) error {
	err := s.SetRunningState(ctx, state.ID, state.RunningProgress, state.ExecutorNodeID)
	if err != nil {
		s.logger.Error("更新为运行状态失败",
			elog.Int64("taskID", state.TaskID),
			elog.String("taskName", state.TaskName),
			elog.Any("state", state),
			elog.FieldErr(err))
		return err
	}
	return nil
}

func (s *executionService) updateRetryState(ctx context.Context, execution domain.TaskExecution, state domain.ExecutionState) error {
	// 计算出下次重试时间
	retryStrategy, _ := retry.NewRetry(execution.Task.RetryConfig.ToRetryComponentConfig())
	duration, shouldRetry := retryStrategy.NextWithRetries(int32(execution.RetryCount + 1))

	if !shouldRetry {
		// NOTE: 达到最大重试次数,状态更新交由消费者统一处理,这里只返回标记错误
		return errs.ErrExecutionMaxRetriesExceeded
	}

	// 还可以重试:计算下次重试时间并更新重试计数
	execution.NextRetryTime = time.Now().Add(duration).UnixMilli()
	execution.RetryCount++

	err := s.UpdateRetryResult(ctx,
		state.ID,
		execution.RetryCount,
		execution.NextRetryTime,
		execution.Status,
		state.Status,
		state.RunningProgress,
		time.Now().UnixMilli(),
		execution.Task.ScheduleParams,
		state.ExecutorNodeID)
	if err != nil {
		s.logger.Error("更新执行计划重试结果失败",
			elog.Int64("taskID", execution.Task.ID),
			elog.String("taskName", execution.Task.Name),
			elog.Any("result", state),
			elog.FieldErr(err))
		return err
	}

	s.logger.Info("更新重试状态成功",
		elog.Int64("taskID", execution.Task.ID),
		elog.String("taskName", execution.Task.Name),
		elog.Any("state", state))
	return nil
}

func (s *executionService) updateState(ctx context.Context, execution domain.TaskExecution, state domain.ExecutionState) error {
	updated, err := s.UpdateScheduleResult(ctx,
		state.ID,
		[]domain.TaskExecutionStatus{execution.Status},
		state.Status,
		state.RunningProgress,
		time.Now().UnixMilli(),
		execution.Task.ScheduleParams,
		state.ExecutorNodeID,
		state.TaskResult)
	if err != nil {
		s.logger.Error("更新调度结果失败",
			elog.Int64("taskID", execution.Task.ID),
			elog.String("taskName", execution.Task.Name),
			elog.Any("state", state),
			elog.FieldErr(err))
		return err
	}
	if !updated {
		return errs.ErrInvalidTaskExecutionStatus
	}
	s.logger.Info("更新调度状态成功",
		elog.Int64("taskID", execution.Task.ID),
		elog.String("taskName", execution.Task.Name),
		elog.Any("state", state))
	return nil
}

func (s *executionService) sendCompletedEvent(ctx context.Context, state domain.ExecutionState,
	execution domain.TaskExecution) error {
	if !state.Status.IsTerminalStatus() {
		return errs.ErrInvalidTaskExecutionStatus
	}
	err := s.producer.Produce(ctx, event.Event{
		ExecID:         execution.ID,
		ScheduleNodeID: execution.Task.ScheduleNodeID,
		ExecNodeId:     execution.ExecutorNodeID,
		ExecStatus:     state.Status,
		TaskID:         execution.Task.ID,
		Name:           execution.Task.Name,
		TaskResult:     state.TaskResult,
		Source:         execution.Source,
		RequestID:      execution.RequestID,
	})
	if err != nil {
		return fmt.Errorf("发送任务完成事件失败: %w", err)
	}
	return nil
}

func (s *executionService) ListByTaskID(ctx context.Context, taskID int64, offset, limit int) ([]domain.TaskExecution, int64, error) {
	return s.repo.ListByTaskID(ctx, taskID, offset, limit)
}

// broadcastExecutionEvent 异步获取最新执行记录并广播
func (s *executionService) broadcastExecutionEvent(id int64) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		exec, err := s.FindByID(ctx, id)
		if err != nil {
			s.logger.Error("广播执行事件时获取记录失败", elog.Int64("id", id), elog.FieldErr(err))
			return
		}
		if exec.Task.ID <= 0 {
			return
		}

		evt := sse.TaskExecutionEvent{
			ID:              exec.ID,
			TaskID:          exec.Task.ID,
			TaskName:        exec.Task.Name,
			StartTime:       exec.StartTime,
			EndTime:         exec.EndTime,
			Status:          exec.Status.String(),
			RunningProgress: exec.RunningProgress,
			ExecutorNodeId:  exec.ExecutorNodeID,
			TaskResult:      exec.TaskResult,
			CTime:           exec.CTime,
		}

		// 广播给该任务的特定订阅者
		s.events.Executions.Broadcast(exec.Task.ID, evt)
	}()
}
