package event

import (
	"context"

	executionevent "github.com/Duke1616/etask/internal/event/execution"
	"github.com/Duke1616/etask/pkg/mpx"
	"github.com/ecodeclub/mq-api"
)

//go:generate go tool mockgen -source=./producer.go -package=eventmocks -destination=./mocks/publisher.mock.go -typed
//go:generate go tool mockgen -package=eventmocks -destination=./mocks/mq.mock.go -typed github.com/ecodeclub/mq-api MQ,Producer

// ExecutionEventPublisher 按派发维度发布有序执行事件。
type ExecutionEventPublisher interface {
	// PublishLogs 按派发 ID 发布一批有序的增量日志。
	PublishLogs(ctx context.Context, batch executionevent.LogBatch) error
	// PublishFinished 发布执行终态及日志器未成功发送的尾部日志。
	PublishFinished(ctx context.Context, finished executionevent.Finished) error
}

type executionEventPublisher struct {
	producer *mqx.GeneralProducer[executionevent.Event]
}

// NewExecutionEventPublisher 创建 Agent 执行事件发布器。
func NewExecutionEventPublisher(q mq.MQ) (ExecutionEventPublisher, error) {
	producer, err := mqx.NewGeneralProducer[executionevent.Event](q, executionevent.EventTopic)
	if err != nil {
		return nil, err
	}
	return &executionEventPublisher{producer: producer}, nil
}

func (p *executionEventPublisher) PublishLogs(ctx context.Context, batch executionevent.LogBatch) error {
	return p.publish(ctx, executionevent.NewLogBatchEvent(batch))
}

func (p *executionEventPublisher) PublishFinished(ctx context.Context, finished executionevent.Finished) error {
	return p.publish(ctx, executionevent.NewFinishedEvent(finished))
}

func (p *executionEventPublisher) publish(ctx context.Context, event executionevent.Event) error {
	if err := event.Validate(); err != nil {
		return err
	}
	return p.producer.ProduceKeyed(ctx, []byte(event.DispatchID), event)
}
