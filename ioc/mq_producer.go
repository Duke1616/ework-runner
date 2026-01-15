package ioc

import (
	"github.com/Duke1616/ework-runner/internal/event"
	"github.com/ecodeclub/mq-api"
)

func InitCompleteProducer(q mq.MQ) event.CompleteProducer {
	producer, err := q.Producer("")
	if err != nil {
		panic(err)
	}
	return event.NewCompleteProducer(producer)
}
