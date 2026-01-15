package invoker

import (
	"context"

	"github.com/Duke1616/ework-runner/internal/domain"
)

type Invoker interface {
	Name() string
	// Run 执行任务，返回执行结果
	Run(ctx context.Context, execution domain.TaskExecution) (domain.ExecutionState, error)
	// Prepare 返回业务总数量
	Prepare(ctx context.Context, execution domain.TaskExecution) (map[string]string, error)
}
