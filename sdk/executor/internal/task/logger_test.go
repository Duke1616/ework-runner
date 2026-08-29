package task

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

type memoryLogger struct {
	sync.Mutex
	messages []string
}

func (m *memoryLogger) Log(format string, args ...any) {
	m.Lock()
	defer m.Unlock()
	m.messages = append(m.messages, fmt.Sprintf(format, args...))
}

func (m *memoryLogger) Close() {}

func TestMaskingExecutionLogger(t *testing.T) {
	testCases := []struct {
		name     string
		masks    []string
		addMasks []string
		logInput string
		expected string
	}{
		{
			name:     "基础敏感信息替换",
			masks:    []string{"secret-password-123"},
			logInput: "user password is secret-password-123, done",
			expected: "user password is [MASKED], done",
		},
		{
			name:     "多组敏感信息同时脱敏",
			masks:    []string{"token-abc", "ak-xyz"},
			logInput: "token=token-abc&ak=ak-xyz",
			expected: "token=[MASKED]&ak=[MASKED]",
		},
		{
			name:     "动态添加掩码",
			masks:    []string{"initial-mask"},
			addMasks: []string{"dynamic-secret"},
			logInput: "initial-mask dynamic-secret plain-text",
			expected: "[MASKED] [MASKED] plain-text",
		},
		{
			name:     "无敏感信息时不影响普通日志",
			masks:    []string{"secret"},
			logInput: "normal task log message",
			expected: "normal task log message",
		},
		{
			name:     "长敏感词优先于短敏感词脱敏（防止子串截断泄漏）",
			masks:    []string{"token", "token_secret_998877"},
			logInput: "full=token_secret_998877 short=token",
			expected: "full=[MASKED] short=[MASKED]",
		},
		{
			name:     "空掩码与重复掩码安全清洗",
			masks:    []string{"", "token-x", "token-x", ""},
			logInput: "value is token-x",
			expected: "value is [MASKED]",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mem := &memoryLogger{}
			logger := newMaskingExecutionLogger(mem, tc.masks)

			if len(tc.addMasks) > 0 {
				if maskable, ok := logger.(interface{ AddMasks(...string) }); ok {
					maskable.AddMasks(tc.addMasks...)
				}
			}

			logger.Log("%s", tc.logInput)
			logger.Close()

			mem.Lock()
			defer mem.Unlock()
			require.Len(t, mem.messages, 1)
			require.Equal(t, tc.expected, mem.messages[0])
		})
	}
}
