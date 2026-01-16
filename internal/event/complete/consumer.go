package complete

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Duke1616/ework-runner/internal/domain"
	"github.com/Duke1616/ework-runner/internal/event"
	"github.com/Duke1616/ework-runner/internal/service/acquirer"
	"github.com/Duke1616/ework-runner/internal/service/task"
	"github.com/ecodeclub/mq-api"
)

const (
	number100 = 100
	number0   = 0
)

type Consumer struct {
	// 更新
	execSvc task.ExecutionService
	taskSvc task.Service
	acquire acquirer.TaskAcquirer
}

func NewConsumer(execSvc task.ExecutionService,
	taskSvc task.Service,
	acquirer acquirer.TaskAcquirer,
) *Consumer {
	return &Consumer{
		taskSvc: taskSvc,
		execSvc: execSvc,
		acquire: acquirer,
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
	var err error
	if evt.ExecStatus.IsSuccess() {
		err = c.execSvc.UpdateScheduleResult(ctx, evt.ExecID, domain.TaskExecutionStatusSuccess, number100, time.Now().UnixMilli(), nil, "")
	} else {
		err = c.execSvc.UpdateScheduleResult(ctx, evt.ExecID, domain.TaskExecutionStatusFailed, number0, time.Now().UnixMilli(), nil, "")
	}
	if err != nil {
		return err
	}
	t, err := c.taskSvc.UpdateNextTime(ctx, evt.TaskID)
	if err != nil {
		return err
	}

	// 只有状态还是 PREEMPTED 的任务才需要释放
	// 一次性任务已经变为 INACTIVE，不需要释放
	if t.Status == domain.TaskStatusPreempted {
		return c.acquire.Release(ctx, evt.TaskID, evt.ScheduleNodeID)
	}

	return nil
}
