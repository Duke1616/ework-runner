package mqx

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ecodeclub/mq-api"
)

type Producer[T any] interface {
	Produce(ctx context.Context, evt T) error
}

type GeneralProducer[T any] struct {
	producer mq.Producer
	topic    string
}

func NewGeneralProducer[T any](q mq.MQ, topic string) (*GeneralProducer[T], error) {
	p, err := q.Producer(topic)
	return &GeneralProducer[T]{
		producer: p,
		topic:    topic,
	}, err
}

func (p *GeneralProducer[T]) Produce(ctx context.Context, evt T) error {
	return p.produce(ctx, nil, evt)
}

// ProduceKeyed 使用业务键发布消息，保证同一键的事件进入同一 Kafka 分区。
func (p *GeneralProducer[T]) ProduceKeyed(ctx context.Context, key []byte, evt T) error {
	return p.produce(ctx, key, evt)
}

func (p *GeneralProducer[T]) produce(ctx context.Context, key []byte, evt T) error {
	data, err := json.Marshal(&evt)
	if err != nil {
		return fmt.Errorf("序列化失败: %w", err)
	}

	msg := &mq.Message{
		Value: data,
		Key:   key,
	}
	// 自动将租户及业务上下文注入 Kafka 消息头部
	InjectContext(ctx, msg)

	_, err = p.producer.Produce(ctx, msg)
	if err != nil {
		return fmt.Errorf("向topic=%s发送event=%#v失败: %w", p.topic, evt, err)
	}
	return nil
}
