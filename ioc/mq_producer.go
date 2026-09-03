package ioc

import (
	"github.com/Duke1616/etask/internal/event"
	"github.com/ecodeclub/mq-api"
)

func InitCompleteProducer(q mq.MQ) event.CompleteProducer {
	producer, err := q.Producer("task_execution_complete_events")
	if err != nil {
		panic(err)
	}
	return event.NewCompleteProducer(producer)
}
