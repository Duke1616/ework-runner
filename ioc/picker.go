package ioc

import (
	"github.com/Duke1616/ecmdb/internal/service/picker"
)

// InitExecutorNodePicker 初始化执行节点选择器
func InitExecutorNodePicker() picker.ExecutorNodePicker {
	// 创建并返回选择器分发器
	return picker.NewBasePicker()
}
