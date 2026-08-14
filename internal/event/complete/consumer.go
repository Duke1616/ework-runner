package complete

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/event"
	"github.com/Duke1616/etask/internal/service/acquirer"
	"github.com/Duke1616/etask/internal/service/notification"
	"github.com/Duke1616/etask/internal/service/task"
	"github.com/Duke1616/etask/internal/sse"
	"github.com/ecodeclub/mq-api"
	"github.com/gotomicro/ego/core/elog"
)

const (
	number100 = 100
	number0   = 0
)

type Consumer struct {
	// 更新
	execSvc  task.ExecutionService
	taskSvc  task.Service
	acquire  acquirer.TaskAcquirer
	events   *sse.Hubs
	notifier notification.CompletionNotifier
	logger   *elog.Component
}

func NewConsumer(execSvc task.ExecutionService,
	taskSvc task.Service,
	acquirer acquirer.TaskAcquirer,
	events *sse.Hubs,
	notifier notification.CompletionNotifier,
) *Consumer {
	return &Consumer{
		taskSvc:  taskSvc,
		execSvc:  execSvc,
		acquire:  acquirer,
		events:   events,
		notifier: notifier,
		logger:   elog.DefaultLogger.With(elog.FieldComponentName("event.complete")),
	}
}

func (c *Consumer) Consume(ctx context.Context, message *mq.Message) error {
	var evt event.Event
	err := json.Unmarshal(message.Value, &evt)
	if err != nil {
		return fmt.Errorf("序列化失败 %w", err)
	}

	return c.handleTask(ctx, evt)
}

func (c *Consumer) handleTask(ctx context.Context, evt event.Event) error {
	status, progress, err := completionState(evt.ExecStatus)
	if err != nil {
		return err
	}
	updated, err := c.execSvc.UpdateScheduleResult(ctx, evt.ExecID,
		domain.NonTerminalTaskExecutionStatuses(), status,
		progress, time.Now().UnixMilli(), nil, evt.ExecNodeId, evt.TaskResult)
	if err != nil {
		return err
	}
	// 重复或迟到的完成事件没有抢到状态迁移，不再重复推进任务和释放抢占。
	if !updated {
		return nil
	}
	// 外部工作流执行没有 etask 正式任务，只需持久化执行终态。
	if evt.TaskID <= 0 {
		return nil
	}

	// 计算下次运行时间
	t, err := c.taskSvc.UpdateNextTime(ctx, evt.TaskID)
	if err != nil {
		return err
	}
	c.notifyCompletion(ctx, evt, t, status)

	// 只有状态还是 PREEMPTED 的任务才需要释放
	// 定时任务由 Release 广播最终的 ACTIVE 状态和下次触发时间。
	if t.Status == domain.TaskStatusPreempted {
		return c.acquire.Release(ctx, evt.TaskID, evt.ScheduleNodeID)
	}

	// 一次性任务已经变为 COMPLETED，不会再经过 Release，需要直接广播终态。
	c.events.Tasks.Broadcast(t.TenantID, sse.TaskStatusEvent{
		TaskID:   t.ID,
		Status:   t.Status.String(),
		NextTime: t.NextTime,
		Version:  t.Version,
	})
	return nil
}

// notifyCompletion 投递命中的任务执行终态通知；通知失败只记录日志，不影响任务状态收敛。
func (c *Consumer) notifyCompletion(ctx context.Context, evt event.Event, t domain.Task,
	status domain.TaskExecutionStatus) {
	if c.notifier == nil {
		return
	}
	rule, ok := t.EnabledNotificationRule(status)
	if !ok {
		return
	}

	execution, err := c.execSvc.FindByID(ctx, evt.ExecID)
	if err != nil {
		c.logger.Error("查询任务执行通知快照失败",
			elog.Int64("taskID", evt.TaskID),
			elog.Int64("executionID", evt.ExecID),
			elog.FieldErr(err))
		return
	}
	if err = c.notifier.Notify(ctx, rule, execution); err != nil {
		c.logger.Error("投递任务执行通知失败",
			elog.Int64("taskID", evt.TaskID),
			elog.Int64("executionID", evt.ExecID),
			elog.String("status", status.String()),
			elog.FieldErr(err))
	}
}

func completionState(status domain.TaskExecutionStatus) (domain.TaskExecutionStatus, int32, error) {
	switch {
	case status.IsSuccess():
		return domain.TaskExecutionStatusSuccess, number100, nil
	case status.IsCancelled():
		return domain.TaskExecutionStatusCancelled, number0, nil
	case status.IsFailed(), status.IsFailedRetryable(), status.IsFailedRescheduled():
		return domain.TaskExecutionStatusFailed, number0, nil
	default:
		return domain.TaskExecutionStatusUnknown, number0,
			fmt.Errorf("非法完成状态: %s", status)
	}
}
