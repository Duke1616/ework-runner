package ioc

import (
	"context"
	"fmt"

	"github.com/Duke1616/etask/internal/event/complete"
	executionevent "github.com/Duke1616/etask/internal/event/execution"
	"github.com/Duke1616/etask/internal/service/acquirer"
	"github.com/Duke1616/etask/internal/service/notification"
	"github.com/Duke1616/etask/internal/service/task"
	internalSSE "github.com/Duke1616/etask/internal/sse"
	mqx "github.com/Duke1616/etask/pkg/mpx"
	"github.com/ecodeclub/mq-api"
)

func InitCompleteEventConsumer(q mq.MQ,
	taskSvc task.Service,
	execSvc task.ExecutionService,
	acquire acquirer.TaskAcquirer,
	events *internalSSE.Hubs,
	notifier notification.CompletionNotifier,
) *CompleteConsumer {
	topic := "task_execution_complete_events"
	group := "reporter"
	con := mqx.NewConsumer(name(topic, group), q, topic)
	comConsumer := complete.NewConsumer(execSvc, taskSvc, acquire, events, notifier)
	return &CompleteConsumer{
		com:      con,
		Consumer: comConsumer,
	}
}

// InitAgentEventConsumer 创建 Scheduler 侧 Agent 事件消费者。
func InitAgentEventConsumer(q mq.MQ, executions task.ExecutionService) *AgentEventConsumer {
	consumer := mqx.NewConsumer(name(executionevent.EventTopic, "scheduler"), q, executionevent.EventTopic)
	return &AgentEventConsumer{consumer: consumer, handler: executionevent.NewEventConsumer(executions)}
}

// AgentEventConsumer 负责启动执行事件消费循环。
type AgentEventConsumer struct {
	consumer *mqx.Consumer
	handler  *executionevent.EventConsumer
}

// Start 启动 Agent 结果消费循环。
func (c *AgentEventConsumer) Start(ctx context.Context) {
	if err := c.consumer.Start(ctx, c.handler.Consume); err != nil {
		panic(err)
	}
}

type CompleteConsumer struct {
	*complete.Consumer
	com *mqx.Consumer
}

func (c *CompleteConsumer) Start(ctx context.Context) {
	err := c.com.Start(ctx, c.Consume)
	if err != nil {
		panic(err)
	}
}

func name(eventName, group string) string {
	return fmt.Sprintf("%s-%s", eventName, group)
}
