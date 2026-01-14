package event

import (
	"context"
	"encoding/json"

	"github.com/ecodeclub/mq-api"
)

type CompleteProducer interface {
	Produce(ctx context.Context, evt Event) error
}

type completeProducer struct {
	producer mq.Producer
}

func NewCompleteProducer(producer mq.Producer) CompleteProducer {
	return &completeProducer{
		producer: producer,
	}
}

func (c *completeProducer) Produce(ctx context.Context, evt Event) error {
	val, err := json.Marshal(evt)
	if err != nil {
		return err
	}
	_, err = c.producer.Produce(ctx, &mq.Message{
		Value: val,
	})
	return err
}
