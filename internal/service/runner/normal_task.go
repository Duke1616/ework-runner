package runner

import (
	"context"
	"fmt"
	"time"

	"github.com/Duke1616/ework-runner/internal/domain"
	"github.com/Duke1616/ework-runner/internal/event"
	"github.com/Duke1616/ework-runner/internal/service/acquirer"
	"github.com/Duke1616/ework-runner/internal/service/invoker"
	"github.com/Duke1616/ework-runner/internal/service/task"
	"github.com/Duke1616/ework-runner/pkg/grpc/balancer"
	"github.com/gotomicro/ego/core/elog"
)

var _ Runner = &NormalTaskRunner{}

type NormalTaskRunner struct {
	nodeID       string                 // 当前调度节点ID
	taskSvc      task.Service           // 任务服务
	execSvc      task.ExecutionService  // 任务执行服务
	taskAcquirer acquirer.TaskAcquirer  // 任务抢占器
	invoker      invoker.Invoker        // 这里一般来说就是 invoker.Dispatcher
	producer     event.CompleteProducer // 任务完成事件生产者

	logger *elog.Component
}

func NewNormalTaskRunner(
	nodeID string,
	taskSvc task.Service,
	execSvc task.ExecutionService,
	taskAcquirer acquirer.TaskAcquirer,
	invoker invoker.Invoker,
	producer event.CompleteProducer,
) *NormalTaskRunner {
	return &NormalTaskRunner{
		nodeID:       nodeID,
		taskSvc:      taskSvc,
		execSvc:      execSvc,
		taskAcquirer: taskAcquirer,
		invoker:      invoker,
		producer:     producer,
		logger:       elog.DefaultLogger.With(elog.FieldComponentName("execute.NormalTaskRunner")),
	}
}

func (s *NormalTaskRunner) Run(ctx context.Context, task domain.Task) error {
	// 抢占任务
	acquiredTask, err := s.acquireTask(ctx, task)
	if err != nil {
		s.logger.Error("任务抢占失败",
			elog.Int64("taskID", task.ID),
			elog.String("taskName", task.Name),
			elog.FieldErr(err))
		return err
	}

	return s.handleNormalTask(ctx, acquiredTask)
}

// acquireTask 抢占任务
func (s *NormalTaskRunner) acquireTask(ctx context.Context, task domain.Task) (domain.Task, error) {
	// 抢占任务
	acquiredTask, err := s.taskAcquirer.Acquire(ctx, task.ID, task.Version, s.nodeID)
	if err != nil {
		return domain.Task{}, fmt.Errorf("任务抢占失败: %w", err)
	}
	// 抢占成功
	return acquiredTask, nil
}

func (s *NormalTaskRunner) handleNormalTask(ctx context.Context, task domain.Task) error {
	// 抢占成功，立即创建TaskExecution记录
	execution, err := s.execSvc.Create(ctx, domain.TaskExecution{
		Task: task,
		// 可以认为开始执行了，防止执行节点直接返回"终态"状态Failed，Success等
		StartTime: time.Now().UnixMilli(),
		Status:    domain.TaskExecutionStatusPrepare,
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

	// 抢占和创建都成功，异步触发任务
	go func() {
		// 执行任务
		state, err1 := s.invoker.Run(ctx, execution)
		if err1 != nil {
			s.logger.Error("执行器执行任务失败", elog.FieldErr(err))
			return
		}

		err1 = s.execSvc.UpdateState(ctx, state)
		if err1 != nil {
			s.logger.Error("正常调度任务失败",
				elog.Any("execution", execution),
				elog.Any("state", state),
				elog.FieldErr(err1))
		}
	}()
	return nil
}

// releaseTask 释放任务
func (s *NormalTaskRunner) releaseTask(ctx context.Context, task domain.Task) {
	if err := s.taskAcquirer.Release(ctx, task.ID, s.nodeID); err != nil {
		s.logger.Error("释放任务失败",
			elog.Int64("taskID", task.ID),
			elog.String("taskName", task.Name),
			elog.FieldErr(err))
	}
}

func (s *NormalTaskRunner) WithSpecificNodeIDContext(ctx context.Context, executorNodeID string) context.Context {
	if executorNodeID != "" {
		return balancer.WithSpecificNodeID(ctx, executorNodeID)
	}
	return ctx
}

// Retry 重试
func (s *NormalTaskRunner) Retry(ctx context.Context, execution domain.TaskExecution) error {
	// 抢占和创建都成功，异步触发任务
	go func() {
		// 执行任务，并在 context 中设置要排除的执行节点 ID，避免重调度到同一个节点
		state, err1 := s.invoker.Run(s.WithExcludedNodeIDContext(ctx, execution.ExecutorNodeID), execution)
		if err1 != nil {
			s.logger.Error("执行器执行任务失败", elog.FieldErr(err1))
			return
		}

		err1 = s.execSvc.UpdateState(ctx, state)
		if err1 != nil {
			s.logger.Error("重试任务失败",
				elog.Any("execution", execution),
				elog.Any("state", state),
				elog.FieldErr(err1))
		}
	}()
	return nil
}

func (s *NormalTaskRunner) WithExcludedNodeIDContext(ctx context.Context, executorNodeID string) context.Context {
	if executorNodeID != "" {
		return balancer.WithExcludedNodeID(ctx, executorNodeID)
	}
	return ctx
}

// Reschedule 重新调度
func (s *NormalTaskRunner) Reschedule(ctx context.Context, execution domain.TaskExecution) error {
	// 抢占和创建都成功，异步触发任务
	go func() {
		// 执行任务，并在 context 中设置要指定的执行节点ID
		state, err1 := s.invoker.Run(s.WithSpecificNodeIDContext(ctx, execution.ExecutorNodeID), execution)
		if err1 != nil {
			s.logger.Error("执行器执行任务失败", elog.FieldErr(err1))
			return
		}

		err1 = s.execSvc.UpdateState(ctx, state)
		if err1 != nil {
			s.logger.Error("重调度任务失败",
				elog.Any("execution", execution),
				elog.Any("state", state),
				elog.FieldErr(err1))
		}
	}()
	return nil
}
