package task

// 本文件集中实现 Context 的并发安全结果管理。

import (
	"encoding/json"
	"fmt"
	"maps"
)

// AddResult 将一组字段合并到任务结果。
func (c *Context) AddResult(data map[string]any) {
	c.resLock.Lock()
	defer c.resLock.Unlock()
	for key, value := range data {
		c.results[key] = value
	}
}

// SetResult 设置一个任务结果字段。
func (c *Context) SetResult(key string, value any) {
	c.resLock.Lock()
	defer c.resLock.Unlock()
	c.results[key] = value
}

// SetResults 使用给定字段替换当前任务结果。
func (c *Context) SetResults(data map[string]any) {
	c.resLock.Lock()
	defer c.resLock.Unlock()
	c.results = maps.Clone(data)
}

// ResultJSON 返回序列化后的任务结果；没有结果时返回空字符串。
func (c *Context) ResultJSON() string {
	value, err := c.Result()
	if err != nil {
		c.SystemLogger().Error("序列化任务结果失败", "error", err)
		return ""
	}
	return value
}

// Result 返回序列化后的任务结果，并将不可序列化值作为执行错误暴露给引擎。
func (c *Context) Result() (string, error) {
	c.resLock.RLock()
	defer c.resLock.RUnlock()
	if len(c.results) == 0 {
		return "", nil
	}
	data, err := json.Marshal(c.results)
	if err != nil {
		return "", fmt.Errorf("序列化任务结果失败: %w", err)
	}
	return string(data), nil
}
