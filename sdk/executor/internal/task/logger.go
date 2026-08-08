package task

// 本文件实现任务日志脱敏和空实现。

import (
	"fmt"
	"strings"
	"sync"
)

// ExecutionLogger 定义用户可见的任务执行日志写入和关闭行为。
type ExecutionLogger interface {
	// Log 记录一条支持格式化参数的任务日志。
	Log(format string, args ...any)
	// Close 刷新剩余日志并释放后台资源。
	Close()
}

type maskingExecutionLogger struct {
	next  ExecutionLogger
	mu    sync.RWMutex
	masks []string
}

func newMaskingExecutionLogger(next ExecutionLogger, masks []string) ExecutionLogger {
	return &maskingExecutionLogger{next: next, masks: append([]string(nil), masks...)}
}

func (l *maskingExecutionLogger) Log(format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	l.mu.RLock()
	for _, mask := range l.masks {
		message = strings.ReplaceAll(message, mask, "[MASKED]")
	}
	l.mu.RUnlock()
	l.next.Log("%s", message)
}

func (l *maskingExecutionLogger) AddMasks(masks ...string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, mask := range masks {
		if mask == "" || containsString(l.masks, mask) {
			continue
		}
		l.masks = append(l.masks, mask)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func (l *maskingExecutionLogger) Close() {
	l.next.Close()
}

type noopExecutionLogger struct{}

func (noopExecutionLogger) Log(string, ...any) {}
func (noopExecutionLogger) Close()             {}

type noopSystemLogger struct{}

func (noopSystemLogger) Debug(string, ...any) {}
func (noopSystemLogger) Info(string, ...any)  {}
func (noopSystemLogger) Warn(string, ...any)  {}
func (noopSystemLogger) Error(string, ...any) {}
