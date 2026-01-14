package picker

import (
	"context"

	"github.com/Duke1616/ecmdb/internal/domain"
)

// ExecutorNodePicker 是执行节点选择器的通用接口。
// 任何实现该接口的类型都可以根据特定逻辑选择一个最优的执行节点。
type ExecutorNodePicker interface {
	// Name 返回选择器的可读名称，主要用于日志和监控。
	Name() string
	// Pick 根据 task 的调度策略选择一个最优的执行节点。
	// 如果没有可用的节点或发生错误，将返回错误。
	Pick(ctx context.Context, task domain.Task) (nodeID string, err error)
}
