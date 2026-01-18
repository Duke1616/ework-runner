package ioc

import (
	"context"
	"fmt"

	"github.com/Duke1616/ework-runner/internal/event/complete"
	"github.com/Duke1616/ework-runner/internal/service/acquirer"
	"github.com/Duke1616/ework-runner/internal/service/task"
	mqx "github.com/Duke1616/ework-runner/pkg/mpx"
	"github.com/ecodeclub/mq-api"
)

func InitCompleteEventConsumer(q mq.MQ,
	taskSvc task.Service,
	execSvc task.ExecutionService,
	acquire acquirer.TaskAcquirer,
) *CompleteConsumer {
	topic := "complete_topic"
	group := "reporter"
	con := mqx.NewConsumer(name(topic, group), q, topic)
	comConsumer := complete.NewConsumer(execSvc, taskSvc, acquire)
	return &CompleteConsumer{
		com:      con,
		Consumer: comConsumer,
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
