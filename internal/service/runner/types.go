package runner

import (
	"context"

	"github.com/Duke1616/ecmdb/internal/domain"
)

type Runner interface {
	// Run 运行任务
	Run(ctx context.Context, task domain.Task) error
	// Retry 重试任务的一次执行
	Retry(ctx context.Context, execution domain.TaskExecution) error
	// Reschedule 重调度任务的一次执行
	Reschedule(ctx context.Context, execution domain.TaskExecution) error
}
