package dispatcher

import (
	"context"
	"fmt"
	"time"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/service/acquirer"
	"github.com/Duke1616/etask/internal/service/invoker"
	"github.com/Duke1616/etask/internal/service/task"
	"github.com/Duke1616/etask/pkg/grpc/balancer"
	"github.com/gotomicro/ego/core/elog"
	"golang.org/x/sync/semaphore"
)

var _ Dispatcher = &TaskDispatcher{}

// TaskDispatcher 负责抢占任务、创建执行记录并触发实际调用。
type TaskDispatcher struct {
	nodeID       string
	execSvc      task.ExecutionService
	taskAcquirer acquirer.TaskAcquirer
	invoker      invoker.Invoker
	routes       RoutePlanner
	invocations  *semaphore.Weighted
	tokenTimeout time.Duration

	logger *elog.Component
}

// Config 控制当前 Scheduler 节点发起执行调用的并发量。
type Config struct {
	MaxConcurrentTasks  int           `mapstructure:"max_concurrent_tasks" yaml:"max_concurrent_tasks"`
	TokenAcquireTimeout time.Duration `mapstructure:"token_acquire_timeout" yaml:"token_acquire_timeout"`
}

// Validate 校验并发控制配置。
func (c Config) Validate() error {
	if c.MaxConcurrentTasks <= 0 {
		return fmt.Errorf("max_concurrent_tasks 必须大于 0")
	}
	if c.TokenAcquireTimeout <= 0 {
		return fmt.Errorf("token_acquire_timeout 必须大于 0")
	}
	return nil
}

type invocationPolicy struct {
	failurePrefix string
	releaseTask   bool
}

var (
	initialInvocation    = invocationPolicy{failurePrefix: "Invocation failed", releaseTask: true}
	retryInvocation      = invocationPolicy{failurePrefix: "Invocation failed during retry", releaseTask: true}
	rescheduleInvocation = invocationPolicy{failurePrefix: "Invocation failed during reschedule"}
)

// NewTaskDispatcher 创建任务派发器。
func NewTaskDispatcher(
	nodeID string,
	execSvc task.ExecutionService,
	taskAcquirer acquirer.TaskAcquirer,
	invoker invoker.Invoker,
	routes RoutePlanner,
	config Config,
) *TaskDispatcher {
	var invocations *semaphore.Weighted
	if config.MaxConcurrentTasks > 0 {
		invocations = semaphore.NewWeighted(int64(config.MaxConcurrentTasks))
	}
	return &TaskDispatcher{
		nodeID:       nodeID,
		execSvc:      execSvc,
		taskAcquirer: taskAcquirer,
		invoker:      invoker,
		routes:       routes,
		invocations:  invocations,
		tokenTimeout: config.TokenAcquireTimeout,
		logger:       elog.DefaultLogger.With(elog.FieldComponentName("dispatcher.TaskDispatcher")),
	}
}

// Run 抢占最新任务快照，完成路由规划后创建并派发执行记录。
func (s *TaskDispatcher) Run(ctx context.Context, task domain.Task) error {
	// 父任务的抢占租约超时后可能再次出现在调度列表中。若其旧 execution
	// 仍未结束，跳过本轮调度，等待旧 execution 自己收敛，避免重复创建记录。
	if task.Status == domain.TaskStatusPreempted && s.execSvc != nil {
		active, err := s.execSvc.HasNonTerminalByTaskID(ctx, task.ID)
		if err != nil {
			return fmt.Errorf("检查任务未完成执行记录失败: %w", err)
		}
		if active {
			s.logger.Debug("任务已有未完成执行记录，跳过重复调度",
				elog.Int64("taskID", task.ID), elog.String("taskName", task.Name))
			return nil
		}
	}

	// 先抢占并取得数据库最新快照，避免基于过期配置选择节点和执行模式。
	acquiredTask, err := s.acquireTask(ctx, task)
	if err != nil {
		s.logger.Error("任务抢占失败",
			elog.Int64("taskID", task.ID),
			elog.String("taskName", task.Name),
			elog.FieldErr(err))
		return err
	}

	// 路由失败时必须释放抢占，否则任务会一直停留在 PREEMPTED。
	route, err := s.routes.Plan(ctx, acquiredTask)
	if err != nil {
		s.releaseTask(ctx, acquiredTask)
		return fmt.Errorf("规划任务派发路由失败: %w", err)
	}
	return s.handleNormalTask(route.Context(ctx), route.Task, route.Execution)
}

// acquireTask 抢占任务
func (s *TaskDispatcher) acquireTask(ctx context.Context, task domain.Task) (domain.Task, error) {
	// 抢占任务
	acquiredTask, err := s.taskAcquirer.Acquire(ctx, task.ID, task.Version, s.nodeID)
	if err != nil {
		return domain.Task{}, fmt.Errorf("任务抢占失败: %w", err)
	}
	// 抢占成功
	return acquiredTask, nil
}

// handleNormalTask 根据执行模式创建执行记录；PULL 模式由 Agent 后续主动拉取。
func (s *TaskDispatcher) handleNormalTask(ctx context.Context, task domain.Task,
	route domain.ExecutionRoute) error {
	// 判断是否为拉取模式
	isPullMode := task.ExecMode.IsPull()

	initStatus := domain.TaskExecutionStatusPrepare
	if isPullMode {
		initStatus = domain.TaskExecutionStatusWaitingPull
	}

	// 抢占成功，立即创建TaskExecution记录
	execution, err := s.execSvc.Create(ctx, domain.TaskExecution{
		Task:  task,
		Route: route,
		// 可以认为开始执行了，防止执行节点直接返回"终态"状态Failed，Success等
		StartTime: time.Now().UnixMilli(),
		Status:    initStatus,
	})
	if err != nil {
		s.logger.Error("创建任务执行记录失败",
			elog.Int64("taskID", task.ID),
			elog.String("taskName", task.Name),
			elog.FieldErr(err))
		// 释放任务
		s.releaseTask(ctx, task)
		return err
	}

	// 如果是 PULL 模式，直接返回，不必做主动推送
	if isPullMode {
		s.logger.Info("任务已进入拉取队列，等待 Agent 主动拉取",
			elog.Int64("task_id", execution.Task.ID),
			elog.Int64("execution_id", execution.ID))
		return nil
	}

	return s.invokeAsync(ctx, execution, initialInvocation)
}

func (s *TaskDispatcher) invokeAsync(ctx context.Context, execution domain.TaskExecution,
	policy invocationPolicy) error {
	if err := s.acquireInvocationSlot(ctx); err != nil {
		s.markInvocationFailed(ctx, execution, policy.failurePrefix, err)
		if policy.releaseTask {
			s.releaseTask(ctx, execution.Task)
		}
		return err
	}
	go func() {
		defer s.releaseInvocationSlot()
		state, err := s.invoker.Run(ctx, execution)
		if err != nil {
			s.logger.Error("执行器调用失败",
				elog.Int64("task_id", execution.Task.ID),
				elog.Int64("execution_id", execution.ID),
				elog.String("task_name", execution.Task.Name),
				elog.FieldErr(err))
			s.markInvocationFailed(ctx, execution, policy.failurePrefix, err)
			if policy.releaseTask {
				s.releaseTask(ctx, execution.Task)
			}
			return
		}
		if err = s.execSvc.UpdateState(ctx, state); err != nil {
			s.logger.Error("保存执行结果失败",
				elog.Int64("execution_id", execution.ID), elog.FieldErr(err))
		}
	}()
	return nil
}

func (s *TaskDispatcher) acquireInvocationSlot(ctx context.Context) error {
	if s.invocations == nil {
		return nil
	}
	acquireCtx, cancel := context.WithTimeout(ctx, s.tokenTimeout)
	defer cancel()
	if err := s.invocations.Acquire(acquireCtx, 1); err != nil {
		return fmt.Errorf("等待任务执行令牌失败: %w", err)
	}
	return nil
}

func (s *TaskDispatcher) releaseInvocationSlot() {
	if s.invocations != nil {
		s.invocations.Release(1)
	}
}

func (s *TaskDispatcher) markInvocationFailed(ctx context.Context, execution domain.TaskExecution,
	prefix string, invocationErr error) {
	if err := s.execSvc.UpdateState(ctx, domain.ExecutionState{
		ID: execution.ID, TaskID: execution.Task.ID,
		Status:     domain.TaskExecutionStatusFailed,
		TaskResult: fmt.Sprintf("%s: %v", prefix, invocationErr),
	}); err != nil {
		s.logger.Error("更新调用失败状态失败", elog.FieldErr(err))
	}
}

// releaseTask 释放任务
func (s *TaskDispatcher) releaseTask(ctx context.Context, task domain.Task) {
	if err := s.taskAcquirer.Release(ctx, task.ID, s.nodeID); err != nil {
		s.logger.Error("释放任务失败",
			elog.Int64("taskID", task.ID),
			elog.String("taskName", task.Name),
			elog.FieldErr(err))
	}
}

// WithSpecificNodeIDContext 将指定执行节点写入 gRPC 负载均衡上下文。
func (s *TaskDispatcher) WithSpecificNodeIDContext(ctx context.Context, executorNodeID string) context.Context {
	if executorNodeID != "" {
		return balancer.WithSpecificNodeID(ctx, executorNodeID)
	}
	return ctx
}

// Retry 重试
func (s *TaskDispatcher) Retry(ctx context.Context, execution domain.TaskExecution) error {
	if execution.Route.DispatchMode.IsPull() {
		return s.execSvc.RequeuePull(ctx, execution.ID)
	}
	claimed, err := s.claimForDispatch(ctx, execution, domain.TaskExecutionStatusFailedRetryable)
	if err != nil {
		return fmt.Errorf("抢占重试执行记录失败: %w", err)
	}
	if !claimed {
		// 其他 Scheduler 已经抢到，或记录已被推进；这是正常的并发结果。
		return nil
	}
	return s.invokeAsync(s.WithExcludedNodeIDContext(ctx, execution.ExecutorNodeID), execution, retryInvocation)
}

// WithExcludedNodeIDContext 将需要排除的执行节点写入 gRPC 负载均衡上下文。
func (s *TaskDispatcher) WithExcludedNodeIDContext(ctx context.Context, executorNodeID string) context.Context {
	if executorNodeID != "" {
		return balancer.WithExcludedNodeID(ctx, executorNodeID)
	}
	return ctx
}

// Reschedule 重新调度
func (s *TaskDispatcher) Reschedule(ctx context.Context, execution domain.TaskExecution) error {
	if execution.Route.DispatchMode.IsPull() {
		return s.execSvc.RequeuePull(ctx, execution.ID)
	}
	claimed, err := s.claimForDispatch(ctx, execution, domain.TaskExecutionStatusFailedRescheduled)
	if err != nil {
		return fmt.Errorf("抢占重调度执行记录失败: %w", err)
	}
	if !claimed {
		return nil
	}
	return s.invokeAsync(s.WithSpecificNodeIDContext(ctx, execution.ExecutorNodeID), execution,
		rescheduleInvocation)
}

// claimForDispatch 将失败态原子迁移到 PREPARE，避免多个 Scheduler 同时再次调用同一 execution。
// Executor 回调随后会把 PREPARE 推进到 RUNNING 或终态。
func (s *TaskDispatcher) claimForDispatch(ctx context.Context, execution domain.TaskExecution,
	expected domain.TaskExecutionStatus) (bool, error) {
	if s.execSvc == nil {
		return false, fmt.Errorf("执行服务未配置")
	}
	return s.execSvc.UpdateScheduleResult(ctx, execution.ID, []domain.TaskExecutionStatus{expected},
		domain.TaskExecutionStatusPrepare, execution.RunningProgress, 0,
		execution.Task.ScheduleParams, "", "")
}
