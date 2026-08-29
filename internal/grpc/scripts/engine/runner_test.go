package engine

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/Duke1616/etask/sdk/executor"
	"github.com/stretchr/testify/require"
)

type memoryExecutionLogger struct {
	sync.Mutex
	lines []string
}

func (m *memoryExecutionLogger) Log(format string, args ...any) {
	m.Lock()
	defer m.Unlock()
	m.lines = append(m.lines, fmt.Sprintf(format, args...))
}

func (m *memoryExecutionLogger) Close() {}

func TestMergeEnvironment(t *testing.T) {
	testCases := []struct {
		name      string
		base      []string
		overrides []string
		want      []string
	}{
		{name: "覆盖同名变量", base: []string{"A=old", "B=keep"}, overrides: []string{"A=new"}, want: []string{"B=keep", "A=new"}},
		{name: "追加新变量", base: []string{"A=old"}, overrides: []string{"B=new"}, want: []string{"A=old", "B=new"}},
		{name: "空覆盖保持原值", base: []string{"A=old"}, want: []string{"A=old"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, MergeEnvironment(tc.base, tc.overrides))
		})
	}
}

func TestStreamOutput(t *testing.T) {
	testCases := []struct {
		name        string
		input       string
		maxLineSize int
		assert      func(t *testing.T, logger *memoryExecutionLogger)
	}{
		{
			name:        "正常读取多行日志",
			input:       "hello world\nline 2\n",
			maxLineSize: 1024,
			assert: func(t *testing.T, logger *memoryExecutionLogger) {
				logger.Lock()
				defer logger.Unlock()
				require.Len(t, logger.lines, 2)
				require.Equal(t, "hello world", logger.lines[0])
				require.Equal(t, "line 2", logger.lines[1])
			},
		},
		{
			name:        "超长日志行截断并输出截断提示",
			input:       strings.Repeat("a", 50) + "\n",
			maxLineSize: 20,
			assert: func(t *testing.T, logger *memoryExecutionLogger) {
				logger.Lock()
				defer logger.Unlock()
				require.Len(t, logger.lines, 2)
				require.Equal(t, strings.Repeat("a", 20), logger.lines[0])
				require.Contains(t, logger.lines[1], "日志行超过 20 字节，剩余内容已截断")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			logger := &memoryExecutionLogger{}
			task := executor.NewContext(executor.ContextOptions{
				Context:         t.Context(),
				ExecutionLogger: logger,
			})
			var wait sync.WaitGroup
			wait.Add(1)
			streamOutput(task, bytes.NewBufferString(tc.input), tc.maxLineSize, &wait)
			wait.Wait()
			tc.assert(t, logger)
		})
	}
}

func TestStreamResult(t *testing.T) {
	testCases := []struct {
		name    string
		input   string
		maximum int64
		assert  func(t *testing.T, task *executor.Context)
	}{
		{
			name:    "多段 JSON 连续读取与结果合并",
			input:   `{"code": 0}{"status": "success"}`,
			maximum: 1024,
			assert: func(t *testing.T, task *executor.Context) {
				resJSON := task.ResultJSON()
				require.Contains(t, resJSON, `"code":0`)
				require.Contains(t, resJSON, `"status":"success"`)
			},
		},
		{
			name:    "超过最大大小限制的对象被丢弃",
			input:   `{"small": 1}{"huge": "` + strings.Repeat("x", 200) + `"}`,
			maximum: 50,
			assert: func(t *testing.T, task *executor.Context) {
				resJSON := task.ResultJSON()
				require.Contains(t, resJSON, `"small":1`)
				require.NotContains(t, resJSON, "huge")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			task := executor.NewContext(executor.ContextOptions{
				Context: t.Context(),
			})
			var wait sync.WaitGroup
			wait.Add(1)
			streamResult(task, bytes.NewBufferString(tc.input), tc.maximum, &wait)
			wait.Wait()
			tc.assert(t, task)
		})
	}
}
