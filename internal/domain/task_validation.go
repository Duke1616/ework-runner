package domain

import "fmt"

// Validate 校验不依赖外部服务的任务定义约束。
func (t *Task) Validate() error {
	if t.Program != nil {
		if err := t.Program.Validate(); err != nil {
			return fmt.Errorf("程序配置非法: %w", err)
		}
	}
	return t.ValidateNotificationRules()
}
