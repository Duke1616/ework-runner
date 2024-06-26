package event

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Duke1616/ecmdb/internal/worker/internal/domain"
	"github.com/Duke1616/ecmdb/internal/worker/internal/service"
	"log/slog"

	"github.com/ecodeclub/mq-api"
)

type WorkerConsumer struct {
	consumer mq.Consumer
	svc      service.Service
}

func NewWorkerConsumer(q mq.MQ, svc service.Service, topic string) (*WorkerConsumer, error) {
	groupID := "runner"

	consumer, err := q.Consumer(topic, groupID)
	if err != nil {
		return nil, err
	}
	return &WorkerConsumer{
		consumer: consumer,
		svc:      svc,
	}, nil
}

func (c *WorkerConsumer) Start(ctx context.Context) {
	go func() {
		for {
			err := c.Consume(ctx)
			if err != nil {
				slog.Error("同步事件失败", err)
			}
		}
	}()
}

func (c *WorkerConsumer) Consume(ctx context.Context) error {
	cm, err := c.consumer.Consume(ctx)
	if err != nil {
		return fmt.Errorf("获取消息失败: %w", err)
	}

	var evt domain.Message
	if err = json.Unmarshal(cm.Value, &evt); err != nil {
		return fmt.Errorf("解析消息失败: %w", err)
	}

	if err = c.svc.Receive(ctx, evt); err != nil {
		slog.Error("执行任务失败", err)
	}

	return err
}

func (c *WorkerConsumer) Stop(_ context.Context) error {
	return c.consumer.Close()
}
