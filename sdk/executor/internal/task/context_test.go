package task

// Context 测试覆盖输入快照和结果隔离。

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestContext(t *testing.T) {
	type state struct {
		params   map[string]string
		metadata map[string]string
		context  *Context
	}
	testCases := []struct {
		name       string
		before     func(t *testing.T, state *state)
		after      func(t *testing.T, state *state)
		assertions func(t *testing.T, state *state)
	}{
		{
			name: "复制输入并解析参数绑定",
			before: func(_ *testing.T, current *state) {
				current.params = map[string]string{"code": "raw", "count": "2"}
				current.metadata = map[string]string{"code": "test"}
				current.context = NewContext(ContextOptions{
					Context: context.Background(), Task: TaskInfo{ExecutionID: 1, TaskID: 2, Name: "task", Handler: "shell"},
					Params: current.params, Metadata: current.metadata,
					Parameters: []Parameter{{Key: "code", Bindings: map[string]Binding{
						"test": &BindingOption{Resolver: func(_ *Context, value string) (string, error) { return "resolved-" + value, nil }},
					}}},
					ExecutionLogger: &executionLoggerStub{},
				})
				current.params["code"] = "changed"
				current.metadata["code"] = "changed"
			},
			assertions: func(t *testing.T, current *state) {
				value, err := current.context.GetResolvedParam("code")
				require.NoError(t, err)
				require.Equal(t, "resolved-raw", value)
				require.Equal(t, 2, current.context.ParamInt("count"))
				require.Equal(t, int64(1), current.context.ExecutionID())
			},
		},
		{
			name: "结果替换不持有外部 map",
			before: func(_ *testing.T, current *state) {
				current.context = NewContext(ContextOptions{ExecutionLogger: &executionLoggerStub{}})
				result := map[string]any{"status": "ok"}
				current.context.SetResults(result)
				result["status"] = "changed"
				current.context.AddResult(map[string]any{"count": 1})
			},
			assertions: func(t *testing.T, current *state) {
				require.JSONEq(t, `{"status":"ok","count":1}`, current.context.ResultJSON())
			},
		},
		{
			name: "制品具名层不暴露共享 map",
			before: func(_ *testing.T, current *state) {
				current.context = NewContext(ContextOptions{ExecutionLogger: &executionLoggerStub{}})
				input := ArtifactRoots{Named: map[string]string{"ops_common": "/cache/ops"}}
				current.context.SetArtifactRoots(input)
				input.Named["ops_common"] = "/changed/input"
			},
			assertions: func(t *testing.T, current *state) {
				roots := current.context.ArtifactRoots()
				require.Equal(t, "/cache/ops", roots.Named["ops_common"])
				roots.Named["ops_common"] = "/changed/output"
				require.Equal(t, "/cache/ops", current.context.ArtifactRoots().Named["ops_common"])
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			current := &state{}
			if tc.before != nil {
				tc.before(t, current)
			}
			if tc.after != nil {
				defer tc.after(t, current)
			}
			tc.assertions(t, current)
		})
	}
}

func TestContextReportProgress(t *testing.T) {
	reporter := &progressReporterStub{}
	ctx := NewContext(ContextOptions{
		Context:  t.Context(),
		Task:     TaskInfo{ExecutionID: 10, TaskID: 20, Name: "task", ExecutorNodeID: "node-1"},
		Progress: reporter, ExecutionLogger: &executionLoggerStub{},
	})
	require.NoError(t, ctx.ReportProgress(120))
	require.Equal(t, int32(100), reporter.progress)
	require.Equal(t, int64(10), reporter.task.ExecutionID)
}

func TestContextResultRejectsUnsupportedValue(t *testing.T) {
	ctx := NewContext(ContextOptions{ExecutionLogger: &executionLoggerStub{}})
	ctx.SetResult("invalid", func() {})
	_, err := ctx.Result()
	require.ErrorContains(t, err, "序列化任务结果失败")
}

type executionLoggerStub struct{}

func (*executionLoggerStub) Log(string, ...any) {}
func (*executionLoggerStub) Close()             {}

var _ ExecutionLogger = (*executionLoggerStub)(nil)

type progressReporterStub struct {
	task     TaskInfo
	progress int32
}

func (r *progressReporterStub) ReportProgress(_ context.Context, task TaskInfo, progress int32) error {
	r.task = task
	r.progress = progress
	return nil
}

var _ ProgressReporter = (*progressReporterStub)(nil)
