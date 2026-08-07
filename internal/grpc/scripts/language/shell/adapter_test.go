package shell

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Duke1616/etask/internal/grpc/scripts/engine"
	"github.com/stretchr/testify/require"
)

type workspaceStub struct {
	root string
	code string
}

func (w *workspaceStub) ProgramRoot() string   { return w.root }
func (w *workspaceStub) EntryPoint() string    { return w.code }
func (w *workspaceStub) Environment() []string { return os.Environ() }
func (w *workspaceStub) Close() error          { return nil }
func (w *workspaceStub) WriteFile(name string, content []byte, mode os.FileMode) (string, error) {
	path := filepath.Join(w.root, name)
	return path, os.WriteFile(path, content, mode)
}

func TestAdapterPrepare(t *testing.T) {
	type state struct {
		workspace *workspaceStub
	}
	testCases := []struct {
		name      string
		variables string
		before    func(t *testing.T, state *state)
		after     func(t *testing.T, state *state)
		wantError string
		assert    func(t *testing.T, state *state, command engine.PreparedCommand)
	}{
		{
			name:      "危险字符保持原值且不会执行命令替换",
			variables: `[{"key":"MESSAGE","value":"literal $(echo injected) ' quoted"}]`,
			assert: func(t *testing.T, state *state, prepared engine.PreparedCommand) {
				require.Equal(t, "MESSAGE=literal $(echo injected) ' quoted", findEnvironment(prepared.Environment, "MESSAGE"))
				content, err := os.ReadFile(filepath.Join(state.workspace.root, "variables.env"))
				require.NoError(t, err)
				require.Equal(t, "MESSAGE='literal $(echo injected) '\\'' quoted'\n", string(content))
				require.Len(t, prepared.Command.Args, 3)
			},
		},
		{
			name:      "创建参数文件",
			variables: `[{"key":"REGION","value":"cn-shanghai"}]`,
			assert: func(t *testing.T, state *state, prepared engine.PreparedCommand) {
				require.FileExists(t, filepath.Join(state.workspace.root, "args.json"))
				require.FileExists(t, filepath.Join(state.workspace.root, "variables.json"))
				require.Len(t, prepared.Command.Args, 3)
			},
		},
		{name: "拒绝非法变量名", variables: `[{"key":"BAD-NAME","value":"value"}]`, wantError: "变量名非法"},
		{name: "拒绝系统保留变量", variables: `[{"key":"ETASK_SYSTEM_ROOT","value":"override"}]`, wantError: "系统保留前缀"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			current := &state{workspace: &workspaceStub{root: t.TempDir()}}
			current.workspace.code = filepath.Join(current.workspace.root, "task.sh")
			require.NoError(t, os.WriteFile(current.workspace.code, []byte("echo ok\n"), 0o700))
			if tc.before != nil {
				tc.before(t, current)
			}
			if tc.after != nil {
				defer tc.after(t, current)
			}
			prepared, err := New("/bin/bash").Prepare(t.Context(), current.workspace, engine.Input{
				Args: `{}`, Variables: tc.variables,
			})
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
				return
			}
			require.NoError(t, err)
			if tc.assert != nil {
				tc.assert(t, current, prepared)
			}
		})
	}
}

func findEnvironment(environment []string, key string) string {
	prefix := key + "="
	for _, item := range environment {
		if len(item) >= len(prefix) && item[:len(prefix)] == prefix {
			return item
		}
	}
	return ""
}

var _ engine.Workspace = (*workspaceStub)(nil)
