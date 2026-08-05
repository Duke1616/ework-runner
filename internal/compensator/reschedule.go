package compensator

import (
	"context"
	"fmt"
	"time"

	"github.com/Duke1616/etask/internal/service/dispatcher"
	"github.com/Duke1616/etask/internal/service/task"
	"github.com/gotomicro/ego/core/elog"
)

// RescheduleConfig 重调度补偿器配置
type RescheduleConfig struct {
	BatchSize   int           `mapstructure:"batch_size" yaml:"batch_size"`     // 批次大小
	MinDuration time.Duration `mapstructure:"min_duration" yaml:"min_duration"` // 最小等待时间，防止空转
}

// RescheduleCompensator 重调度补偿器
type RescheduleCompensator struct {
	dispatcher dispatcher.Dispatcher
	execSvc    task.ExecutionService
	config     RescheduleConfig
	logger     *elog.Component
}

// NewRescheduleCompensator 创建重调度补偿器
func NewRescheduleCompensator(
	dispatcher dispatcher.Dispatcher,
	execSvc task.ExecutionService,
	config RescheduleConfig,
) *RescheduleCompensator {
	return &RescheduleCompensator{
		dispatcher: dispatcher,
		execSvc:    execSvc,
		config:     config,
		logger:     elog.DefaultLogger.With(elog.FieldComponentName("compensator.reschedule")),
	}
}

// Start 启动补偿器
func (r *RescheduleCompensator) Start(ctx context.Context) {
	r.logger.Info("重调度补偿器启动")

	for {
		select {
		case <-ctx.Done():
			r.logger.Info("重调度补偿器停止")
			return
		default:
			startTime := time.Now()

			err := r.reschedule(ctx)
			if err != nil {
				r.logger.Error("重调度失败", elog.FieldErr(err))
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

// reschedule 执行一轮补偿
func (r *RescheduleCompensator) reschedule(ctx context.Context) error {
	// 查找可重调度的执行记录
	executions, err := r.execSvc.FindReschedulableExecutions(
		ctx,
		r.config.BatchSize,
	)
	if err != nil {
		return fmt.Errorf("查找可重调度任务失败: %w", err)
	}

	if len(executions) == 0 {
		r.logger.Debug("没有找到可重调度的任务")
		return nil
	}

	r.logger.Info("找到可重调度任务", elog.Int("count", len(executions)))

	// 处理每个可重调度的执行
	for i := range executions {
		err = r.dispatcher.Reschedule(ctx, executions[i])
		if err != nil {
			r.logger.Error("重调度失败",
				elog.Int64("executionId", executions[i].ID),
				elog.String("taskName", executions[i].Task.Name),
				elog.FieldErr(err))
			continue
		}
	}
	return nil
}
