package invoker

import (
	"context"

	"github.com/Duke1616/etask/internal/domain"
)

//go:generate go tool mockgen -source=./types.go -package=invokermocks -destination=./mocks/invoker.mock.go -typed

type Invoker interface {
	// Name 返回调用器唯一名称。
	Name() string
	// Run 执行任务，返回执行结果
	Run(ctx context.Context, execution domain.TaskExecution) (domain.ExecutionState, error)
	// Terminate 向本次执行固定的传输目标发送终止信号。
	Terminate(ctx context.Context, execution domain.TaskExecution, reason string) error
}
