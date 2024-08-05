package event

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Duke1616/ecmdb/internal/execute/internal/domain"
	"github.com/Duke1616/ecmdb/internal/execute/internal/service"
	"github.com/ecodeclub/mq-api"
	"github.com/gotomicro/ego/core/elog"
)

type ExecuteConsumer struct {
	consumer mq.Consumer
	producer TaskExecuteResultProducer
	svc      service.Service
	logger   *elog.Component
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
		logger:   elog.DefaultLogger,
	}, nil
}

func (c *ExecuteConsumer) Start(ctx context.Context) {
	go func() {
		for {
			err := c.Consume(ctx)
			if err != nil {
				c.logger.Error("同步事件失败", elog.Any("错误信息: ", err))
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

	c.logger.Info("开始执行任务", elog.Int64("任务ID", evt.TaskId))

	output, status, err := c.svc.Receive(ctx, domain.ExecuteReceive{
		TaskId:    evt.TaskId,
		Language:  evt.Language,
		Code:      evt.Code,
		Args:      string(args),
		Variables: evt.Variables,
	})

	if err != nil {
		c.logger.Error("执行任务失败", elog.Any("错误", err), elog.Any("任务ID", evt.TaskId))
	} else {
		c.logger.Info("执行任务完成", elog.Int64("任务ID", evt.TaskId))
	}

	err = c.producer.Produce(ctx, ExecuteResultEvent{
		TaskId: evt.TaskId,
		Result: output,
		Status: Status(status),
	})

	if err != nil {
		c.logger.Error("发送消息队列失败", elog.Any("错误", err), elog.Any("任务ID", evt.TaskId))
	}

	return err
}

func (c *ExecuteConsumer) Stop(_ context.Context) error {
	return c.consumer.Close()
}
