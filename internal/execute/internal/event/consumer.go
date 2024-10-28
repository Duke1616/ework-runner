package event

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Duke1616/ecmdb/internal/execute/internal/domain"
	"github.com/Duke1616/ecmdb/internal/execute/internal/service"
	"github.com/ecodeclub/mq-api"
	"github.com/gotomicro/ego/core/elog"
	"strings"
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
		TaskId:     evt.TaskId,
		WantResult: c.wantResult(output),
		Result:     output,
		Status:     Status(status),
	})

	if err != nil {
		c.logger.Error("发送消息队列失败", elog.Any("错误", err), elog.Any("任务ID", evt.TaskId))
	}

	return err
}

func (c *ExecuteConsumer) wantResult(output string) string {
	outputStr := strings.TrimSpace(output)
	// 检查输出是否为空
	if outputStr == "" {
		c.logger.Info("No output from command.")
	}

	// 分割输出为多行并过滤掉空行
	lines := strings.Split(outputStr, "\n")
	var validLines []string
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			validLines = append(validLines, line)
		}
	}

	// 检查 validLines 是否为空
	if len(validLines) == 0 {
		c.logger.Info("No valid output lines.")
	}

	// 获取最后一行
	lastLine := validLines[len(validLines)-1]

	return lastLine
}

func (c *ExecuteConsumer) Stop(_ context.Context) error {
	return c.consumer.Close()
}
