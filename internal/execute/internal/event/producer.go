package event

import (
	"context"
	"github.com/Duke1616/ecmdb/pkg/mpx"
	"github.com/ecodeclub/mq-api"
)

type TaskExecuteResultProducer interface {
	Produce(ctx context.Context, evt ExecuteResultEvent) error
}

func NewExecuteResultEventProducer(q mq.MQ) (TaskExecuteResultProducer, error) {
	return mqx.NewGeneralProducer[ExecuteResultEvent](q, ExecuteResultEventName)
}
