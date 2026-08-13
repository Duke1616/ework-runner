package task

import (
	"context"
	"fmt"
	"strings"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/errs"
	poolSvc "github.com/Duke1616/etask/internal/service/pool"
)

const (
	maxOverrideRules   = 100
	maxOverrideOptions = 500
	maxOverrideLength  = 1024
)

func (s *service) validateParamOverrideRules(ctx context.Context, task domain.Task) error {
	if len(task.ParamOverrideRules) == 0 {
		return nil
	}
	if len(task.ParamOverrideRules) > maxOverrideRules {
		return fmt.Errorf("%w: 启动参数覆盖规则不能超过 %d 条", errs.ErrInvalidParameter, maxOverrideRules)
	}
	if task.GrpcConfig == nil || s.bindingSvc == nil {
		return fmt.Errorf("%w: 当前任务没有可覆盖的 Handler 参数", errs.ErrInvalidParameter)
	}
	validKeys, err := s.bindingSvc.RuntimeOverridableParameterKeys(ctx, poolSvc.CheckBindingRequest{
		TenantID: task.TenantID, PoolName: task.GrpcConfig.ServiceName, HandlerName: task.GrpcConfig.HandlerName,
	})
	if err != nil {
		return fmt.Errorf("查询 Handler 可覆盖参数失败: %w", err)
	}

	seen := make(map[string]struct{}, len(task.ParamOverrideRules))
	for _, rule := range task.ParamOverrideRules {
		key := strings.TrimSpace(rule.ParamKey)
		if key == "" || len(key) > 128 {
			return fmt.Errorf("%w: 启动参数覆盖规则的参数名非法", errs.ErrInvalidParameter)
		}
		if _, ok := validKeys[key]; !ok {
			return fmt.Errorf("%w: 参数 %s 未在 Handler 元数据中声明允许启动覆盖", errs.ErrInvalidParameter, key)
		}
		if _, ok := seen[key]; ok {
			return fmt.Errorf("%w: 参数 %s 的覆盖规则重复", errs.ErrInvalidParameter, key)
		}
		seen[key] = struct{}{}
		if err := validateOverrideRule(rule); err != nil {
			return err
		}
	}
	return nil
}

func validateOverrideRule(rule domain.TaskParamOverrideRule) error {
	if len(rule.AllowedModes) == 0 || len(rule.AllowedModes) > 2 {
		return fmt.Errorf("%w: 参数 %s 至少需要一种输入方式", errs.ErrInvalidParameter, rule.ParamKey)
	}
	allowed := make(map[domain.TaskParamInputMode]struct{}, len(rule.AllowedModes))
	for _, mode := range rule.AllowedModes {
		if mode != domain.TaskParamInputModeManual && mode != domain.TaskParamInputModeSelect {
			return fmt.Errorf("%w: 参数 %s 的输入方式 %s 不受支持", errs.ErrInvalidParameter, rule.ParamKey, mode)
		}
		if _, exists := allowed[mode]; exists {
			return fmt.Errorf("%w: 参数 %s 的输入方式重复", errs.ErrInvalidParameter, rule.ParamKey)
		}
		allowed[mode] = struct{}{}
	}
	if _, ok := allowed[rule.DefaultMode]; !ok {
		return fmt.Errorf("%w: 参数 %s 的默认输入方式不在允许范围内", errs.ErrInvalidParameter, rule.ParamKey)
	}
	if _, ok := allowed[domain.TaskParamInputModeSelect]; ok && rule.DefaultMode != domain.TaskParamInputModeSelect {
		return fmt.Errorf("%w: 参数 %s 启用预设选择后必须以预设选择为默认方式", errs.ErrInvalidParameter, rule.ParamKey)
	}
	if _, ok := allowed[domain.TaskParamInputModeSelect]; !ok {
		if rule.SelectConfig != nil {
			return fmt.Errorf("%w: 参数 %s 未启用预设选择", errs.ErrInvalidParameter, rule.ParamKey)
		}
		return nil
	}
	config := rule.SelectConfig
	if config == nil || len(config.Options) == 0 || len(config.Options) > maxOverrideOptions {
		return fmt.Errorf("%w: 参数 %s 的可选项数量必须为 1-%d", errs.ErrInvalidParameter, rule.ParamKey, maxOverrideOptions)
	}
	values := make(map[string]struct{}, len(config.Options))
	for _, option := range config.Options {
		if strings.TrimSpace(option.Label) == "" || strings.TrimSpace(option.Value) == "" ||
			len(option.Label) > 128 || len(option.Value) > maxOverrideLength || strings.Contains(option.Value, ",") {
			return fmt.Errorf("%w: 参数 %s 包含非法可选项", errs.ErrInvalidParameter, rule.ParamKey)
		}
		if _, exists := values[option.Value]; exists {
			return fmt.Errorf("%w: 参数 %s 的可选值 %s 重复", errs.ErrInvalidParameter, rule.ParamKey, option.Value)
		}
		values[option.Value] = struct{}{}
	}
	return nil
}

func validateAndSerializeOverrides(rules []domain.TaskParamOverrideRule,
	overrides map[string]domain.RunParamOverride) (map[string]string, error) {
	byKey := make(map[string]domain.TaskParamOverrideRule, len(rules))
	for _, rule := range rules {
		byKey[rule.ParamKey] = rule
	}
	result := make(map[string]string, len(overrides))
	for key, override := range overrides {
		rule, ok := byKey[key]
		if !ok {
			return nil, fmt.Errorf("%w: 参数 %s 不允许在启动时覆盖", errs.ErrInvalidParameter, key)
		}
		if !containsMode(rule.AllowedModes, override.Mode) {
			return nil, fmt.Errorf("%w: 参数 %s 不允许使用 %s 输入", errs.ErrInvalidParameter, key, override.Mode)
		}
		switch override.Mode {
		case domain.TaskParamInputModeManual:
			if len(override.Value) > maxOverrideLength || len(override.Values) > 0 {
				return nil, fmt.Errorf("%w: 参数 %s 的手动输入值非法", errs.ErrInvalidParameter, key)
			}
			result[key] = override.Value
		case domain.TaskParamInputModeSelect:
			if rule.SelectConfig == nil || override.Value != "" {
				return nil, fmt.Errorf("%w: 参数 %s 的选择值非法", errs.ErrInvalidParameter, key)
			}
			values, err := validateSelectedValues(key, *rule.SelectConfig, override.Values)
			if err != nil {
				return nil, err
			}
			result[key] = strings.Join(values, ",")
		default:
			return nil, fmt.Errorf("%w: 参数 %s 的输入方式非法", errs.ErrInvalidParameter, key)
		}
	}
	return result, nil
}

func containsMode(modes []domain.TaskParamInputMode, target domain.TaskParamInputMode) bool {
	for _, mode := range modes {
		if mode == target {
			return true
		}
	}
	return false
}

func validateSelectedValues(key string, config domain.TaskParamSelectConfig, values []string) ([]string, error) {
	if len(values) == 0 || (!config.Multiple && len(values) != 1) {
		return nil, fmt.Errorf("%w: 参数 %s 的选择数量非法", errs.ErrInvalidParameter, key)
	}
	options := make(map[string]struct{}, len(config.Options))
	for _, option := range config.Options {
		options[option.Value] = struct{}{}
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := options[value]; !ok {
			return nil, fmt.Errorf("%w: 参数 %s 包含未配置的选项 %s", errs.ErrInvalidParameter, key, value)
		}
		if _, duplicate := seen[value]; duplicate {
			return nil, fmt.Errorf("%w: 参数 %s 包含重复选项", errs.ErrInvalidParameter, key)
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result, nil
}
