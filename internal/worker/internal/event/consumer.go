package event

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Duke1616/ecmdb/internal/runner"
	"log/slog"

	"github.com/ecodeclub/mq-api"
)

type RunnerConsumer struct {
	consumer  mq.Consumer
	runnerSvc runner.Service
}

func NewRunnerConsumer(q mq.MQ, runnerSvc runner.Service, topic string) (*RunnerConsumer, error) {
	groupID := "runner"
	consumer, err := q.Consumer(topic, groupID)
	if err != nil {
		return nil, err
	}
	return &RunnerConsumer{
		consumer:  consumer,
		runnerSvc: runnerSvc,
	}, nil
}

func (c *RunnerConsumer) Start(ctx context.Context) {
	go func() {
		for {
			err := c.Consume(ctx)
			if err != nil {
				slog.Error("同步事件失败", err)
			}
		}
	}()
}

func (c *RunnerConsumer) Consume(ctx context.Context) error {
	cm, err := c.consumer.Consume(ctx)
	if err != nil {
		return fmt.Errorf("获取消息失败: %w", err)
	}

	var evt runner.Runner
	if err = json.Unmarshal(cm.Value, &evt); err != nil {
		return fmt.Errorf("解析消息失败: %w", err)
	}

	if err = c.runnerSvc.Start(ctx, evt); err != nil {
		slog.Error("执行任务失败", err)
	}

	return err
}

func (c *RunnerConsumer) Stop(_ context.Context) error {
	return c.consumer.Close()
}
