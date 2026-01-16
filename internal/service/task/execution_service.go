package task

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Duke1616/ework-runner/internal/domain"
	"github.com/Duke1616/ework-runner/internal/errs"
	"github.com/Duke1616/ework-runner/internal/event"
	"github.com/Duke1616/ework-runner/internal/repository"
	"github.com/Duke1616/ework-runner/internal/service/acquirer"
	"github.com/Duke1616/ework-runner/pkg/grpc/registry"
	"github.com/Duke1616/ework-runner/pkg/retry"
	"github.com/gotomicro/ego/core/elog"
	"go.uber.org/multierr"
)

// ExecutionService 任务执行服务接口
type ExecutionService interface {
	// Create 创建任务执行实例
	Create(ctx context.Context, execution domain.TaskExecution) (domain.TaskExecution, error)
	// FindByID 根据ID获取执行实例
	FindByID(ctx context.Context, id int64) (domain.TaskExecution, error)
	// FindRetryableExecutions 查找所有可以重试的执行记录
	// limit: 查询结果数量限制
	FindRetryableExecutions(ctx context.Context, limit int) ([]domain.TaskExecution, error)
	// FindReschedulableExecutions 查找所有可以重调度的执行记录
	FindReschedulableExecutions(ctx context.Context, limit int) ([]domain.TaskExecution, error)
	FindExecutionByTaskIDAndPlanExecID(ctx context.Context, taskID int64, planExecID int64) (domain.TaskExecution, error)
	// FindTimeoutExecutions 查找超时的执行记录
	FindTimeoutExecutions(ctx context.Context, limit int) ([]domain.TaskExecution, error)

	// SetRunningState 设置任务为运行状态并更新进度
	SetRunningState(ctx context.Context, id int64, progress int32, executorNodeID string) error
	// UpdateRunningProgress 更新任务执行进度（仅在RUNNING状态下有效）
	UpdateRunningProgress(ctx context.Context, id int64, progress int32) error
	// UpdateRetryResult 更新重试结果
	UpdateRetryResult(ctx context.Context, id, retryCount, nextRetryTime int64, status domain.TaskExecutionStatus, progress int32, endTime int64, scheduleParams map[string]string, executorNodeID string) error
	// UpdateScheduleResult 更新调度结果
	UpdateScheduleResult(ctx context.Context, id int64, status domain.TaskExecutionStatus, progress int32, endTime int64, scheduleParams map[string]string, executorNodeID string) error

	// HandleReports 处理执行节点上报的执行状态
	HandleReports(ctx context.Context, reports []*domain.Report) error
	// UpdateState 更新执行节点上报的执行状态
	UpdateState(ctx context.Context, state domain.ExecutionState) error
}

type executionService struct {
	nodeID       string
	repo         repository.TaskExecutionRepository
	taskSvc      Service
	taskAcquirer acquirer.TaskAcquirer  // 任务抢占器
	producer     event.CompleteProducer // 任务完成事件生产者
	registry     registry.Registry
	logger       *elog.Component
}

// NewExecutionService 创建任务执行服务实例
func NewExecutionService(
	nodeID string,
	repo repository.TaskExecutionRepository,
	taskSvc Service,
	taskAcquirer acquirer.TaskAcquirer,
	producer event.CompleteProducer,
	registry registry.Registry,
) ExecutionService {
	return &executionService{
		nodeID:       nodeID,
		repo:         repo,
		taskSvc:      taskSvc,
		taskAcquirer: taskAcquirer,
		producer:     producer,
		registry:     registry,
		logger:       elog.DefaultLogger.With(elog.FieldComponentName("service.execution")),
	}
}

func (s *executionService) Create(ctx context.Context, execution domain.TaskExecution) (domain.TaskExecution, error) {
	return s.repo.Create(ctx, execution)
}

func (s *executionService) FindByID(ctx context.Context, id int64) (domain.TaskExecution, error) {
	return s.repo.GetByID(ctx, id)
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
	return s.repo.SetRunningState(ctx, id, progress, executorNodeID)
}

func (s *executionService) UpdateRunningProgress(ctx context.Context, id int64, progress int32) error {
	return s.repo.UpdateRunningProgress(ctx, id, progress)
}

func (s *executionService) UpdateRetryResult(ctx context.Context, id, retryCount, nextRetryTime int64, status domain.TaskExecutionStatus, progress int32, endTime int64, scheduleParams map[string]string, executorNodeID string) error {
	return s.repo.UpdateRetryResult(ctx, id, retryCount, nextRetryTime, status, progress, endTime, scheduleParams, executorNodeID)
}

func (s *executionService) UpdateScheduleResult(ctx context.Context, id int64, status domain.TaskExecutionStatus, progress int32, endTime int64, scheduleParams map[string]string, executorNodeID string) error {
	return s.repo.UpdateScheduleResult(ctx, id, status, progress, endTime, scheduleParams, executorNodeID)
}

func (s *executionService) HandleReports(ctx context.Context, reports []*domain.Report) error {
	if len(reports) == 0 {
		return nil
	}
	s.logger.Debug("开始处理执行状态上报", elog.Int("count", len(reports)))

	var err error
	processedCount := 0
	skippedCount := 0

	for i := range reports {
		err1 := s.UpdateState(ctx, reports[i].ExecutionState)
		if err1 != nil {
			skippedCount++
			s.logger.Error("处理执行节点上报的结果失败",
				elog.Any("result", reports[i].ExecutionState),
				elog.FieldErr(err1))
			// 包装错误，添加上报场景的特定信息
			err = multierr.Append(err,
				fmt.Errorf("处理执行节点上报的结果失败: taskID=%d, executionID=%d: %w",
					reports[i].ExecutionState.TaskID, reports[i].ExecutionState.ID, err1))
			continue
		}
		processedCount++
	}

	// 记录处理统计信息
	s.logger.Info("执行状态上报处理完成",
		elog.Int("total", len(reports)),
		elog.Int("processed", processedCount),
		elog.Int("skipped", skippedCount))
	return err
}

func (s *executionService) UpdateState(ctx context.Context, state domain.ExecutionState) error {
	execution, err := s.FindByID(ctx, state.ID)
	if err != nil {
		return errs.ErrExecutionNotFound
	}

	// 已处于终止状态的的执行记录不允许再进行状态迁移
	if execution.Status.IsTerminalStatus() {
		s.logger.Error("错乱的状态迁移",
			elog.Int64("taskID", execution.Task.ID),
			elog.String("taskName", execution.Task.Name),
			elog.String("currentStatus", execution.Status.String()),
			elog.String("targetStatus", state.Status.String()))
		return errs.ErrInvalidTaskExecutionStatus
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
				s.sendCompletedEvent(ctx, state, execution)
				return nil
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
		// NOTE: 只发送完成事件,由消费者统一更新终止状态,避免重复更新
		s.sendCompletedEvent(ctx, state, execution)
		return nil
	default:
		s.logger.Error("非法上报状态",
			elog.Int64("taskID", execution.Task.ID),
			elog.String("taskName", execution.Task.Name),
			elog.String("currentStatus", execution.Status.String()),
			elog.String("targetStatus", state.Status.String()))
		return errs.ErrInvalidTaskExecutionStatus
	}
}

func (s *executionService) updateRunningProgress(ctx context.Context, state domain.ExecutionState) error {
	err := s.UpdateRunningProgress(ctx, state.ID, state.RunningProgress)
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
	err := s.UpdateScheduleResult(ctx,
		state.ID,
		state.Status,
		state.RunningProgress,
		time.Now().UnixMilli(),
		execution.Task.ScheduleParams,
		state.ExecutorNodeID)
	if err != nil {
		s.logger.Error("更新调度结果失败",
			elog.Int64("taskID", execution.Task.ID),
			elog.String("taskName", execution.Task.Name),
			elog.Any("state", state),
			elog.FieldErr(err))
		return err
	}
	s.logger.Info("更新调度状态成功",
		elog.Int64("taskID", execution.Task.ID),
		elog.String("taskName", execution.Task.Name),
		elog.Any("state", state))
	return nil
}

func (s *executionService) releaseTask(ctx context.Context, task domain.Task) {
	if err := s.taskAcquirer.Release(ctx, task.ID, s.nodeID); err != nil {
		s.logger.Error("释放任务失败",
			elog.Int64("taskID", task.ID),
			elog.String("taskName", task.Name),
			elog.FieldErr(err))
	}
}

func (s *executionService) sendCompletedEvent(ctx context.Context, state domain.ExecutionState, execution domain.TaskExecution) {
	if !state.Status.IsTerminalStatus() {
		// 非终止状态不用做处理
		return
	}
	err := s.producer.Produce(ctx, event.Event{
		ExecID:         execution.ID,
		ScheduleNodeID: execution.Task.ScheduleNodeID,
		ExecStatus:     state.Status,
		TaskID:         execution.Task.ID,
		Name:           execution.Task.Name,
	})
	if err != nil {
		s.logger.Error("发送完成事件失败", elog.Int64("taskID", execution.Task.ID), elog.FieldErr(err))
	}
}
