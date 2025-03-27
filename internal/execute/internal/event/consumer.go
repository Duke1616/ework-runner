package event

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Duke1616/ecmdb/internal/execute/internal/domain"
	"github.com/Duke1616/ecmdb/internal/execute/internal/service"
	"github.com/ecodeclub/mq-api"
	"github.com/gotomicro/ego/core/elog"
	"strings"
	"sync"
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

func (c *ExecuteConsumer) Start(ctx context.Context) <-chan error {
	errChan := make(chan error, 1) // 用于返回启动错误或运行期错误

	go func() {
		defer close(errChan)

		workerCount := 5
		var wg sync.WaitGroup

		// 启动worker协程
		for i := 0; i < workerCount; i++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				c.logger.Info("启动任务工作协程", elog.Int("worker", workerID))

				for {
					select {
					case <-ctx.Done():
						c.logger.Info("工作协程退出", elog.Int("worker", workerID))
						return
					default:
						if err := c.Consume(ctx); err != nil {
							if errors.Is(err, context.Canceled) {
								return // 正常关闭，不记录错误
							}
							c.logger.Error("处理失败",
								elog.Int("worker", workerID),
								elog.FieldErr(err))
							// 可选：将非context错误报告到errChan
							select {
							case errChan <- fmt.Errorf("worker %d 处理失败: %w", workerID, err):
							default: // 避免阻塞，只报告第一个错误
							}
						}
					}
				}
			}(i)
		}

		wg.Wait()
		c.logger.Info("所有工作协程已退出")
	}()

	return errChan
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
		c.logger.Error("No output from command.", elog.String("output", output))
		return `{"status": "Error"}`
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
		c.logger.Error("No valid output lines.", elog.Any("lines", validLines))
		return `{"status": "Error"}`
	}

	// 获取最后一行
	lastLine := validLines[len(validLines)-1]

	return lastLine
}

func (c *ExecuteConsumer) Stop(_ context.Context) error {
	return c.consumer.Close()
}
