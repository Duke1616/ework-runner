package complete

import (
	"context"
	"encoding/json"
	"fmt"

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

func (c *Consumer) handleTask(_ context.Context, _ event.Event) error {
	// 普通的任务,暂时啥也不做
	return nil
}
