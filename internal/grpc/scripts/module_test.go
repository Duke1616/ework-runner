package scripts

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Duke1616/etask/internal/grpc/scripts/engine"
	"github.com/Duke1616/etask/sdk/executor"
	"github.com/stretchr/testify/require"
)

func TestRuntimeHandlersExecuteProjectSource(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("当前环境未安装 python3")
	}
	project := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(project, "check.sh"), []byte(
		`test -f ./check.sh && test -f "$ETASK_PROJECT_ROOT/check.sh"`+"\n"), 0o440))
	require.NoError(t, os.WriteFile(filepath.Join(project, "check.py"), []byte(
		"import os\nimport helper\nassert helper.VALUE == 'ok'\nassert os.path.isfile('check.py')\nassert os.path.isfile(os.path.join(os.environ['ETASK_PROJECT_ROOT'], 'check.py'))\n"), 0o440))
	require.NoError(t, os.WriteFile(filepath.Join(project, "helper.py"), []byte("VALUE = 'ok'\n"), 0o440))
	disabled := false
	runtime, err := NewRuntime(RuntimeConfig{
		WorkspaceDir: t.TempDir(), ShellBinary: "/bin/sh", PythonBinary: python,
		Archive: ArchiveConfig{Enabled: &disabled},
	})
	require.NoError(t, err)
	handlers := make(map[string]executor.TaskHandler)
	for _, handler := range runtime.Handlers() {
		handlers[handler.Name()] = handler
	}
	for _, tc := range []struct{ handler, entryPoint string }{
		{handler: "shell", entryPoint: "check.sh"},
		{handler: "python", entryPoint: "check.py"},
	} {
		t.Run(tc.handler, func(t *testing.T) {
			task := executor.NewContext(executor.ContextOptions{
				Context:         t.Context(),
				Task:            executor.TaskInfo{ExecutionID: 1, Handler: tc.handler},
				Params:          map[string]string{"args": `{}`, "variables": `[]`},
				ExecutionLogger: runtimeExecutionLogger{},
			})
			task.SetProgram(&executor.Program{
				Kind:    executor.ProgramKindProject,
				Project: &executor.ProjectProgram{Root: project, EntryPoint: tc.entryPoint},
			})
			require.NoError(t, handlers[tc.handler].Run(task))
		})
	}
}

type runtimeExecutionLogger struct{}

func (runtimeExecutionLogger) Log(string, ...any) {}
func (runtimeExecutionLogger) Close()             {}

func TestNewRuntime(t *testing.T) {
	testCases := []struct {
		name   string
		config RuntimeConfig
	}{
		{name: "默认配置创建两个处理器"},
		{name: "允许自定义运行目录", config: RuntimeConfig{WorkspaceDir: t.TempDir()}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			runtime, err := NewRuntime(tc.config)
			require.NoError(t, err)
			handlers := runtime.Handlers()
			require.Len(t, handlers, 2)
			require.Equal(t, "shell", handlers[0].Name())
			require.Equal(t, "python", handlers[1].Name())
			handlers[0] = nil
			require.NotNil(t, runtime.Handlers()[0], "Handlers 应返回副本")
		})
	}
}

func TestRuntimeInitializeOnce(t *testing.T) {
	lifecycle := &runtimeLifecycleFake{}
	adapter := &runtimeAdapterFake{}
	runtime := &Runtime{
		adapters: []engine.Adapter{adapter}, workspaces: lifecycle, archiver: lifecycle,
	}

	const callers = 8
	var wg sync.WaitGroup
	errs := make(chan error, callers)
	wg.Add(callers)
	for range callers {
		go func() {
			defer wg.Done()
			errs <- runtime.Initialize()
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	require.Equal(t, int32(1), adapter.validations.Load())
	require.Equal(t, int32(2), lifecycle.validations.Load())
	require.Equal(t, int32(2), lifecycle.prunes.Load())
}

type runtimeLifecycleFake struct {
	validations atomic.Int32
	prunes      atomic.Int32
}

func (f *runtimeLifecycleFake) Create(engine.WorkspaceOptions) (engine.Workspace, error) {
	panic("测试不应创建工作区")
}
func (f *runtimeLifecycleFake) Archive(engine.ArchiveRecord) error { return nil }
func (f *runtimeLifecycleFake) Validate() error {
	f.validations.Add(1)
	return nil
}
func (f *runtimeLifecycleFake) Prune() error {
	f.prunes.Add(1)
	return nil
}

type runtimeAdapterFake struct{ validations atomic.Int32 }

func (f *runtimeAdapterFake) Name() string                   { return "test" }
func (f *runtimeAdapterFake) Description() string            { return "测试解释器" }
func (f *runtimeAdapterFake) Extension() string              { return ".test" }
func (f *runtimeAdapterFake) Metadata() []executor.Parameter { return nil }
func (f *runtimeAdapterFake) Prepare(context.Context, engine.Workspace,
	engine.Input) (engine.PreparedCommand, error) {
	return engine.PreparedCommand{Command: exec.Command("true")}, nil
}
func (f *runtimeAdapterFake) Validate() error {
	f.validations.Add(1)
	return nil
}
