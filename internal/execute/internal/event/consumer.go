package event

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Duke1616/ecmdb/internal/execute/internal/domain"
	"github.com/Duke1616/ecmdb/internal/execute/internal/service"
	"log/slog"

	"github.com/ecodeclub/mq-api"
)

type ExecuteConsumer struct {
	consumer mq.Consumer
	producer TaskExecuteResultProducer
	svc      service.Service
}

func NewExecuteConsumer(q mq.MQ, svc service.Service, topic string, producer TaskExecuteResultProducer) (
	*ExecuteConsumer, error) {
	groupID := "task_receive_execute"
	consumer, err := q.Consumer(topic, groupID)
	if err != nil {
		return nil, err
	}
	return &ExecuteConsumer{
		consumer: consumer,
		producer: producer,
		svc:      svc,
	}, nil
}

func (c *ExecuteConsumer) Start(ctx context.Context) {
	go func() {
		for {
			err := c.Consume(ctx)
			if err != nil {
				slog.Error("同步事件失败", err)
			}
		}
	}()
}

func (c *ExecuteConsumer) Consume(ctx context.Context) error {
	cm, err := c.consumer.Consume(ctx)
	if err != nil {
		return fmt.Errorf("获取消息失败: %w", err)
	}
	var evt ExecuteReceive
	if err = json.Unmarshal(cm.Value, &evt); err != nil {
		return fmt.Errorf("解析消息失败: %w", err)
	}

	// 封转成 Json 数据
	args, err := json.Marshal(evt.Args)
	if err != nil {
		return err
	}

	output, status, err := c.svc.Receive(ctx, domain.ExecuteReceive{
		TaskId:   evt.TaskId,
		Language: evt.Language,
		Code:     evt.Code,
		Args:     string(args),
	})

	if err != nil {
		slog.Error("执行任务失败", slog.Any("错误", err), slog.Any("任务ID", evt.TaskId))
	}

	err = c.producer.Produce(ctx, ExecuteResultEvent{
		TaskId: evt.TaskId,
		Result: output,
		Status: Status(status),
	})

	if err != nil {
		slog.Error("发送消息队列失败", slog.Any("错误", err), slog.Any("任务ID", evt.TaskId))
	}

	return err
}

func (c *ExecuteConsumer) Stop(_ context.Context) error {
	return c.consumer.Close()
}
