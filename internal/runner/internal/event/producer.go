package event

import (
	"context"
	"github.com/Duke1616/ecmdb/pkg/mpx"
	"github.com/ecodeclub/mq-api"
)

type TaskRunnerEventProducer interface {
	Produce(ctx context.Context, evt TaskRunnerEvent) error
}

func NewTaskRunnerEventProducer(q mq.MQ) (TaskRunnerEventProducer, error) {
	return mqx.NewGeneralProducer[TaskRunnerEvent](q, TaskRegisterRunnerEventName)
}
