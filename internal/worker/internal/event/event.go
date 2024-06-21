package event

import (
	"context"
	"github.com/Duke1616/ecmdb/pkg/mpx"
	"github.com/ecodeclub/mq-api"
)

type TaskWorkerEventProducer interface {
	Produce(ctx context.Context, evt WorkerEvent) error
}

func NewTaskWorkerEventProducer(q mq.MQ) (TaskWorkerEventProducer, error) {
	return mqx.NewGeneralProducer[WorkerEvent](q, TaskWorkerEventName)
}
