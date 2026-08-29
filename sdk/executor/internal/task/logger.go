package task

// 本文件实现任务日志脱敏和空实现。

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/samber/lo"
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
	return &maskingExecutionLogger{next: next, masks: cleanMasks(masks)}
}

func cleanMasks(masks []string) []string {
	cleaned := lo.Uniq(lo.Filter(masks, func(m string, _ int) bool {
		return m != ""
	}))
	// 按长度从大到小降序排列，防止短敏感词先替换破坏长敏感词导致子串泄露
	sort.Slice(cleaned, func(i, j int) bool {
		return len(cleaned[i]) > len(cleaned[j])
	})
	return cleaned
}

func (l *maskingExecutionLogger) Log(format string, args ...any) {
	l.mu.RLock()
	masks := l.masks
	// Fast-path: 无敏感词掩码时直接透传输出，避免额外开销
	if len(masks) == 0 {
		l.mu.RUnlock()
		l.next.Log(format, args...)
		return
	}
	message := fmt.Sprintf(format, args...)
	for _, mask := range masks {
		message = strings.ReplaceAll(message, mask, "[MASKED]")
	}
	l.mu.RUnlock()
	l.next.Log("%s", message)
}

func (l *maskingExecutionLogger) AddMasks(masks ...string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	hasNew := false
	for _, mask := range masks {
		if mask == "" || slices.Contains(l.masks, mask) {
			continue
		}
		l.masks = append(l.masks, mask)
		hasNew = true
	}
	if hasNew {
		// 追加新敏感词后重新按长度降序排序
		sort.Slice(l.masks, func(i, j int) bool {
			return len(l.masks[i]) > len(l.masks[j])
		})
	}
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
