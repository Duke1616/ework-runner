package ioc

import (
	"github.com/Duke1616/ework-runner/internal/service/picker"
	"github.com/Duke1616/ework-runner/pkg/grpc/registry"
)

// InitExecutorNodePicker 初始化执行节点选择器
func InitExecutorNodePicker(reg registry.Registry) picker.ExecutorNodePicker {
	// NOTE: 已统一使用 service 前缀
	return picker.NewRandomPicker(reg)
}
