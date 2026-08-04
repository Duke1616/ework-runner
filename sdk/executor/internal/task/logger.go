package task

// 本文件实现任务日志脱敏和空实现。

import (
	"fmt"
	"strings"
)

// Logger 任务日志记录器接口
type Logger interface {
	// Log 记录一条支持格式化参数的任务日志。
	Log(format string, args ...any)
	// Close 刷新剩余日志并释放后台资源。
	Close()
}

type maskingTaskLogger struct {
	next  Logger
	masks []string
}

func newMaskingTaskLogger(next Logger, masks []string) Logger {
	if len(masks) == 0 {
		return next
	}
	return &maskingTaskLogger{next: next, masks: masks}
}

func (l *maskingTaskLogger) Log(format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	for _, mask := range l.masks {
		message = strings.ReplaceAll(message, mask, "[MASKED]")
	}
	l.next.Log("%s", message)
}

func (l *maskingTaskLogger) Close() {
	l.next.Close()
}

type noopTaskLogger struct{}

func (noopTaskLogger) Log(string, ...any) {}
func (noopTaskLogger) Close()             {}

type noopSystemLogger struct{}

func (noopSystemLogger) Debug(string, ...any) {}
func (noopSystemLogger) Info(string, ...any)  {}
func (noopSystemLogger) Warn(string, ...any)  {}
func (noopSystemLogger) Error(string, ...any) {}
