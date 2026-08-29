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
	"github.com/Duke1616/etask/internal/grpc/scripts/language/ansible"
	"github.com/Duke1616/etask/internal/grpc/scripts/language/ansible/connection"
	"github.com/Duke1616/etask/internal/grpc/scripts/language/python"
	"github.com/Duke1616/etask/internal/grpc/scripts/language/shell"
	"github.com/Duke1616/etask/sdk/executor"
	"github.com/stretchr/testify/require"
)

func TestRuntimeHandlersExecuteProjectSource(t *testing.T) {
	pythonBinary, err := exec.LookPath("python3")
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
		WorkspaceDir: t.TempDir(),
		Shell:        shell.Config{Enabled: true, Binary: "/bin/sh"},
		Python:       python.Config{Enabled: true, Binary: pythonBinary},
		Archive:      ArchiveConfig{Enabled: &disabled},
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

func TestRuntimeAnsibleHandlerExecutesProjectSource(t *testing.T) {
	project := t.TempDir()
	entryPoint := filepath.Join(project, "playbooks", "deploy.yml")
	role := filepath.Join(project, "roles", "deploy", "tasks", "main.yml")
	require.NoError(t, os.MkdirAll(filepath.Dir(entryPoint), 0o750))
	require.NoError(t, os.MkdirAll(filepath.Dir(role), 0o750))
	require.NoError(t, os.WriteFile(entryPoint, []byte("---\n"), 0o440))
	require.NoError(t, os.WriteFile(role, []byte("---\n"), 0o440))

	binary := filepath.Join(t.TempDir(), "ansible-playbook")
	require.NoError(t, os.WriteFile(binary, []byte(`#!/bin/sh
set -eu
test "$1" = "--extra-vars"
extra_vars="${2#@}"
test -f "$extra_vars"
test -f "$3"
test -f "./roles/deploy/tasks/main.yml"
grep -q '"environment":"staging"' "$extra_vars"
`), 0o700))
	disabled := false
	runtime, err := NewRuntime(RuntimeConfig{
		WorkspaceDir: t.TempDir(),
		Sandbox:      SandboxConfig{Mode: SandboxModeOff},
		Ansible:      ansible.Config{Enabled: true, Binary: binary},
		Archive:      ArchiveConfig{Enabled: &disabled},
	})
	require.NoError(t, err)
	require.NoError(t, runtime.Initialize())

	var handler executor.TaskHandler
	for _, candidate := range runtime.Handlers() {
		if candidate.Name() == "ansible" {
			handler = candidate
			break
		}
	}
	require.NotNil(t, handler)
	task := executor.NewContext(executor.ContextOptions{
		Context: t.Context(), Task: executor.TaskInfo{ExecutionID: 2, Handler: "ansible"},
		Variables: &executor.VariableSet{Items: []executor.Variable{
			{Key: "environment", Value: "staging"}, {Key: "ansible_user", Value: "deploy"},
		}},
		ExecutionLogger: runtimeExecutionLogger{},
	})
	task.SetProgram(&executor.Program{
		Kind: executor.ProgramKindProject,
		Project: &executor.ProjectProgram{
			Root: project, EntryPoint: "playbooks/deploy.yml",
		},
	})
	require.NoError(t, handler.Run(task))
}

type runtimeExecutionLogger struct{}

func (runtimeExecutionLogger) Log(string, ...any) {}
func (runtimeExecutionLogger) Close()             {}

func TestNewRuntime(t *testing.T) {
	testCases := []struct {
		name         string
		config       RuntimeConfig
		wantHandlers []string
	}{
		{name: "默认配置不注册处理器"},
		{
			name: "注册全部显式开启的处理器",
			config: RuntimeConfig{
				WorkspaceDir: t.TempDir(),
				Shell:        shell.Config{Enabled: true},
				Python:       python.Config{Enabled: true},
				Ansible:      ansible.Config{Enabled: true},
			},
			wantHandlers: []string{"shell", "python", "ansible"},
		},
		{
			name: "只注册显式开启的处理器",
			config: RuntimeConfig{
				Python: python.Config{Enabled: true},
			},
			wantHandlers: []string{"python"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			runtime, err := NewRuntime(tc.config)
			require.NoError(t, err)
			handlers := runtime.Handlers()
			require.Len(t, handlers, len(tc.wantHandlers))
			for index, name := range tc.wantHandlers {
				require.Equal(t, name, handlers[index].Name())
			}
			if len(handlers) == 3 {
				programHandler, ok := handlers[2].(executor.ProgramHandler)
				require.True(t, ok)
				require.Equal(t, []executor.ProgramKind{executor.ProgramKindProject}, programHandler.ProgramKinds())
			}
			if len(handlers) > 0 {
				handlers[0] = nil
				require.NotNil(t, runtime.Handlers()[0], "Handlers 应返回副本")
			}
		})
	}
}

func TestNewRuntimeIgnoresDisabledAnsibleConfig(t *testing.T) {
	runtime, err := NewRuntime(RuntimeConfig{Ansible: ansible.Config{
		CredentialRoot: "relative/credentials",
		Credentials: map[string]connection.CredentialConfig{
			"production": {Username: "deploy", PrivateKeyFile: "production-key"},
		},
	}})
	require.NoError(t, err)
	require.Empty(t, runtime.Handlers())
}

func TestNewRuntimeRejectsUnsafeAnsibleCredentialConfig(t *testing.T) {
	_, err := NewRuntime(RuntimeConfig{Ansible: ansible.Config{
		Enabled:        true,
		CredentialRoot: "relative/credentials",
		Credentials: map[string]connection.CredentialConfig{
			"production": {Username: "deploy", PrivateKeyFile: "production-key"},
		},
	}})
	require.ErrorContains(t, err, "credential_root 必须是绝对路径")
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

func (f *runtimeAdapterFake) Name() string        { return "test" }
func (f *runtimeAdapterFake) Description() string { return "测试解释器" }
func (f *runtimeAdapterFake) ProgramKinds() []executor.ProgramKind {
	return []executor.ProgramKind{executor.ProgramKindInline}
}
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
