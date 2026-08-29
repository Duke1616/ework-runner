package domain

import (
	"fmt"
	"strings"

	"github.com/Duke1616/etask/internal/pkg/variable"
)

// Validate 校验不依赖外部服务的任务定义约束。
func (t *Task) Validate() error {
	if t.GrpcConfig != nil {
		if err := ValidateVariableItems(t.GrpcConfig.Variables); err != nil {
			return err
		}
	}
	if t.Program != nil {
		if err := t.Program.Validate(); err != nil {
			return fmt.Errorf("程序配置非法: %w", err)
		}
	}
	return t.ValidateNotificationRules()
}

// ValidateVariableItems 校验结构化变量集合，确保名称非空且不重复。
func ValidateVariableItems(items []variable.Item) error {
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		key := strings.TrimSpace(item.Key)
		if key == "" {
			return fmt.Errorf("结构化变量名称不能为空")
		}
		if _, exists := seen[key]; exists {
			return fmt.Errorf("结构化变量 %s 重复", key)
		}
		seen[key] = struct{}{}
	}
	return nil
}
