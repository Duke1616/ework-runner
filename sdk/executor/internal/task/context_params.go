package task

// 本文件集中实现 Context 的参数读取能力。

import (
	"encoding/json"
	"strconv"
)

// Param 返回指定参数的字符串值。
func (c *Context) Param(key string) string {
	return c.params[key]
}

// GetResolvedParam 根据参数选择的绑定模式返回最终值。
func (c *Context) GetResolvedParam(key string) (string, error) {
	rawValue := c.params[key]
	bindingName := c.metadata[key]
	parameter, exists := c.parameters[key]
	if !exists || bindingName == "" {
		return rawValue, nil
	}
	binding, exists := parameter.Bindings[bindingName]
	if !exists {
		return rawValue, nil
	}
	return binding.Resolve(c, rawValue)
}

// ParamInt 返回指定参数的 int 值，非法值返回 0 并记录日志。
func (c *Context) ParamInt(key string) int {
	value, err := strconv.Atoi(c.params[key])
	if err != nil && c.params[key] != "" {
		c.logInvalidParam(key, c.params[key], err)
	}
	return value
}

// ParamInt64 返回指定参数的 int64 值，非法值返回 0 并记录日志。
func (c *Context) ParamInt64(key string) int64 {
	value, err := strconv.ParseInt(c.params[key], 10, 64)
	if err != nil && c.params[key] != "" {
		c.logInvalidParam(key, c.params[key], err)
	}
	return value
}

// ParamBool 返回指定参数的 bool 值，非法值返回 false 并记录日志。
func (c *Context) ParamBool(key string) bool {
	value, err := strconv.ParseBool(c.params[key])
	if err != nil && c.params[key] != "" {
		c.logInvalidParam(key, c.params[key], err)
	}
	return value
}

// BindPayload 将 payload 参数中的 JSON 解析到 target。
func (c *Context) BindPayload(target any) error {
	value := c.params["payload"]
	if value == "" {
		return nil
	}
	return json.Unmarshal([]byte(value), target)
}

func (c *Context) logInvalidParam(key, value string, err error) {
	c.Logger().Warn("任务参数类型转换失败", "key", key, "value", value, "error", err)
}
