package runner

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// ParameterKeyArgs 是外部预览和工作流协议约定的统一脚本入参 Key。
const ParameterKeyArgs = "args"

// MergeParameterDefaults 将 Runner 默认参数转换为执行协议使用的字符串，并应用本次覆盖值。
// 调用方显式传入的空字符串也会保留，用于清除 Runner 默认值。
func MergeParameterDefaults(defaults map[string]json.RawMessage,
	overrides map[string]string) (map[string]string, error) {
	merged := make(map[string]string, len(defaults)+len(overrides))
	for key, raw := range defaults {
		value, err := ParameterDefaultValue(raw)
		if err != nil {
			return nil, fmt.Errorf("Runner 默认参数 %s 非法: %w", key, err)
		}
		merged[key] = value
	}
	for key, value := range overrides {
		merged[key] = value
	}
	return merged, nil
}

// ParameterDefaultValue 把结构化 JSON 值转换为现有 Handler 参数协议使用的字符串。
// JSON 字符串会去掉引号，数字和布尔值保留字面量，数组与对象返回紧凑 JSON。
func ParameterDefaultValue(raw json.RawMessage) (string, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return "", nil
	}
	if !json.Valid(raw) {
		return "", fmt.Errorf("不是合法 JSON")
	}
	if raw[0] == '"' {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return "", err
		}
		return value, nil
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		return "", err
	}
	return compact.String(), nil
}

// ParameterDefaultsBytes 复制默认参数的原始 JSON，供 gRPC 查询接口返回。
func ParameterDefaultsBytes(defaults map[string]json.RawMessage) map[string][]byte {
	if defaults == nil {
		return nil
	}
	result := make(map[string][]byte, len(defaults))
	for key, raw := range defaults {
		result[key] = append([]byte(nil), raw...)
	}
	return result
}
