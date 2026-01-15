package ioc

import (
	"github.com/Duke1616/ework-runner/internal/service/picker"
	"github.com/Duke1616/ework-runner/pkg/grpc/registry"
)

// InitExecutorNodePicker 初始化执行节点选择器
func InitExecutorNodePicker(reg registry.Registry) picker.ExecutorNodePicker {
	// 创建并返回随机选择器
	return picker.NewBasePicker(reg)
}
