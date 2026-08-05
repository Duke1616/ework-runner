package picker

import (
	"context"

	"github.com/Duke1616/etask/internal/domain"
)

//go:generate go tool mockgen -source=./types.go -package=pickermocks -destination=./mocks/picker.mock.go -typed

// ExecutorNodePicker 为 gRPC PUSH 任务选择一个可用执行节点。
type ExecutorNodePicker interface {
	// Pick 根据任务所需服务和 Handler 返回节点 ID。
	Pick(ctx context.Context, task domain.Task) (string, error)
}
