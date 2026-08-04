package engine

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Duke1616/etask/sdk/executor"
	"github.com/gotomicro/ego/core/elog"
	"github.com/stretchr/testify/require"
)

type handlerTestState struct {
	workspace *workspaceFake
	archiver  *archiverFake
	handler   *Handler
	task      *executor.Context
}

func TestHandlerRun(t *testing.T) {
	testCases := []struct {
		name       string
		params     map[string]string
		adapter    adapterFake
		config     Config
		before     func(t *testing.T, state *handlerTestState)
		after      func(t *testing.T, state *handlerTestState)
		wantError  string
		wantFailed bool
	}{
		{
			name:   "成功执行后归档并清理工作区",
			params: map[string]string{"code": "echo ok", "args": `{}`, "variables": `[]`},
			adapter: adapterFake{command: func(ctx context.Context) *exec.Cmd {
				return exec.CommandContext(ctx, "/bin/sh", "-c", "printf ok")
			}},
		},
		{
			name:   "失败执行记录失败归档",
			params: map[string]string{"code": "exit 1", "args": `{}`, "variables": `[]`},
			adapter: adapterFake{command: func(ctx context.Context) *exec.Cmd {
				return exec.CommandContext(ctx, "/bin/sh", "-c", "exit 1")
			}},
			wantError: "退出码非 0", wantFailed: true,
		},
		{
			name:   "输入超限时不创建工作区",
			params: map[string]string{"code": "12345"},
			adapter: adapterFake{command: func(ctx context.Context) *exec.Cmd {
				return exec.CommandContext(ctx, "/bin/sh", "-c", "exit 0")
			}},
			config: Config{MaxCodeSize: 4}, wantError: "代码大小超过限制",
			after: func(t *testing.T, state *handlerTestState) {
				require.False(t, state.workspace.created)
				require.Empty(t, state.archiver.records)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			current := &handlerTestState{
				workspace: &workspaceFake{root: t.TempDir()},
				archiver:  &archiverFake{},
			}
			current.workspace.code = filepath.Join(current.workspace.root, "task.sh")
			config := tc.config
			adapter := tc.adapter
			handler, err := NewHandler(
				config, &adapter, current.workspace, current.archiver, NewHostProcessLauncher(),
			)
			require.NoError(t, err)
			current.handler = handler
			current.task = executor.NewContext(executor.ContextOptions{
				Context: t.Context(), Task: executor.TaskInfo{ExecutionID: 9, TaskID: 1, Name: "test", Handler: "shell"},
				Params: tc.params, Logger: elog.DefaultLogger, TaskLogger: &taskLoggerFake{},
			})
			if tc.before != nil {
				tc.before(t, current)
			}
			if tc.after != nil {
				defer tc.after(t, current)
			}
			runErr := handler.Run(current.task)
			if tc.wantError != "" {
				require.ErrorContains(t, runErr, tc.wantError)
			} else {
				require.NoError(t, runErr)
			}
			if tc.wantError == "代码大小超过限制" {
				return
			}
			require.True(t, current.workspace.closed)
			require.Len(t, current.archiver.records, 1)
			require.Equal(t, tc.wantFailed, current.archiver.records[0].Failed)
		})
	}
}

func TestNewHandlerRejectsMissingProcessLauncher(t *testing.T) {
	_, err := NewHandler(Config{}, &adapterFake{}, &workspaceFake{}, &archiverFake{}, nil)
	require.ErrorContains(t, err, "依赖不能为空")
}

type workspaceFake struct {
	root    string
	code    string
	created bool
	closed  bool
}

func (w *workspaceFake) Create(options WorkspaceOptions) (Workspace, error) {
	w.created = true
	if err := os.WriteFile(w.code, options.Code, 0o700); err != nil {
		return nil, err
	}
	return w, nil
}
func (w *workspaceFake) Prune() error          { return nil }
func (w *workspaceFake) Validate() error       { return nil }
func (w *workspaceFake) Root() string          { return w.root }
func (w *workspaceFake) CodeFile() string      { return w.code }
func (w *workspaceFake) Environment() []string { return os.Environ() }
func (w *workspaceFake) Close() error          { w.closed = true; return nil }
func (w *workspaceFake) WriteFile(name string, content []byte, mode os.FileMode) (string, error) {
	path := filepath.Join(w.root, name)
	return path, os.WriteFile(path, content, mode)
}

type adapterFake struct {
	command func(context.Context) *exec.Cmd
}

func (a *adapterFake) Name() string                   { return "shell" }
func (a *adapterFake) Description() string            { return "测试脚本" }
func (a *adapterFake) Extension() string              { return ".sh" }
func (a *adapterFake) Metadata() []executor.Parameter { return nil }
func (a *adapterFake) Validate() error                { return nil }
func (a *adapterFake) Prepare(ctx context.Context, _ Workspace, _ Input) (PreparedCommand, error) {
	return PreparedCommand{Command: a.command(ctx)}, nil
}

type archiverFake struct {
	records []ArchiveRecord
}

func (a *archiverFake) Archive(record ArchiveRecord) error {
	a.records = append(a.records, record)
	return nil
}
func (a *archiverFake) Prune() error    { return nil }
func (a *archiverFake) Validate() error { return nil }

type taskLoggerFake struct{}

func (taskLoggerFake) Log(string, ...any) {}
func (taskLoggerFake) Close()             {}

var _ WorkspaceFactory = (*workspaceFake)(nil)
var _ Adapter = (*adapterFake)(nil)
var _ Archiver = (*archiverFake)(nil)
