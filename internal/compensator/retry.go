package compensator

import (
	"context"
	"fmt"
	"time"

	"github.com/Duke1616/etask/internal/service/dispatcher"
	"github.com/Duke1616/etask/internal/service/task"
	"github.com/gotomicro/ego/core/elog"
)

// RetryConfig 重试补偿器配置
type RetryConfig struct {
	BatchSize   int           `mapstructure:"batch_size" yaml:"batch_size"`     // 批量处理大小
	MinDuration time.Duration `mapstructure:"min_duration" yaml:"min_duration"` // 最小等待时间，防止空转
}

// RetryCompensator 重试补偿器
type RetryCompensator struct {
	dispatcher dispatcher.Dispatcher
	execSvc    task.ExecutionService
	config     RetryConfig
	logger     *elog.Component
}

// NewRetryCompensator 创建重试补偿器
func NewRetryCompensator(
	dispatcher dispatcher.Dispatcher,
	execSvc task.ExecutionService,
	config RetryConfig,
) *RetryCompensator {
	return &RetryCompensator{
		dispatcher: dispatcher,
		execSvc:    execSvc,
		config:     config,
		logger:     elog.DefaultLogger.With(elog.FieldComponentName("compensator.retry")),
	}
}

// Start 启动补偿器
func (r *RetryCompensator) Start(ctx context.Context) {
	r.logger.Info("重试补偿器启动")

	for {
		select {
		case <-ctx.Done():
			r.logger.Info("重试补偿器停止")
			return
		default:
			startTime := time.Now()

			err := r.retry(ctx)
			if err != nil {
				r.logger.Error("重试失败", elog.FieldErr(err))
			}

			// 防空转：确保最小等待时间
			elapsed := time.Since(startTime)
			if elapsed < r.config.MinDuration {
				select {
				case <-ctx.Done():
					return
				case <-time.After(r.config.MinDuration - elapsed):
				}
			}
		}
	}
}

// retry 执行一轮补偿
func (r *RetryCompensator) retry(ctx context.Context) error {
	// 查找可重试的执行记录
	executions, err := r.execSvc.FindRetryableExecutions(
		ctx,
		r.config.BatchSize,
	)
	if err != nil {
		return fmt.Errorf("查找可重试任务失败: %w", err)
	}

	if len(executions) == 0 {
		r.logger.Debug("没有找到可重试的任务")
		return nil
	}

	r.logger.Info("找到可重试任务", elog.Int("count", len(executions)))

	// 处理每个可重试的执行
	for i := range executions {
		err = r.dispatcher.Retry(ctx, executions[i])
		if err != nil {
			r.logger.Error("重试任务失败",
				elog.Int64("executionId", executions[i].ID),
				elog.String("taskName", executions[i].Task.Name),
				elog.Int64("retryCount", executions[i].RetryCount),
				elog.FieldErr(err))
			continue
		}
	}
	return nil
}
