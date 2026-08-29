package python

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Duke1616/etask/internal/grpc/scripts/engine"
	"github.com/Duke1616/etask/sdk/executor"
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

var _ engine.Workspace = (*workspaceStub)(nil)

func TestAdapter_MetadataAndDefaults(t *testing.T) {
	testCases := []struct {
		name       string
		binary     string
		wantBinary string
	}{
		{name: "指定明确的二进制路径", binary: "/usr/bin/python3", wantBinary: "/usr/bin/python3"},
		{name: "空路径回退到默认 python", binary: "", wantBinary: "python"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			adapter := New(tc.binary)
			require.Equal(t, tc.wantBinary, adapter.binary)
			require.Equal(t, "python", adapter.Name())
			require.Equal(t, ".py", adapter.Extension())
			require.NotEmpty(t, adapter.Description())
			require.True(t, slices.Contains(adapter.ProgramKinds(), executor.ProgramKindInline))
			require.True(t, slices.Contains(adapter.ProgramKinds(), executor.ProgramKindProject))
			require.NotEmpty(t, adapter.Metadata())
		})
	}
}

func TestAdapter_Prepare(t *testing.T) {
	testCases := []struct {
		name      string
		args      string
		variables string
		assert    func(t *testing.T, ws *workspaceStub, prepared engine.PreparedCommand)
	}{
		{
			name:      "正常写入参数和变量文件并注入环境变量",
			args:      `{"limit":10,"dryRun":true}`,
			variables: `[{"key":"ENV","value":"prod"}]`,
			assert: func(t *testing.T, ws *workspaceStub, prepared engine.PreparedCommand) {
				argsContent, err := os.ReadFile(filepath.Join(ws.root, "args.json"))
				require.NoError(t, err)
				require.Equal(t, `{"limit":10,"dryRun":true}`, string(argsContent))

				varsContent, err := os.ReadFile(filepath.Join(ws.root, "variables.json"))
				require.NoError(t, err)
				require.Equal(t, `[{"key":"ENV","value":"prod"}]`, string(varsContent))

				require.Contains(t, prepared.Environment, "ETASK_ARGS_FILE="+filepath.Join(ws.root, "args.json"))
				require.Contains(t, prepared.Environment, "ETASK_VARIABLES_FILE="+filepath.Join(ws.root, "variables.json"))
				require.Equal(t, ws.code, prepared.Command.Args[1])
			},
		},
		{
			name:      "空入参时自动提供默认空 JSON 占位",
			args:      "",
			variables: "",
			assert: func(t *testing.T, ws *workspaceStub, prepared engine.PreparedCommand) {
				argsContent, err := os.ReadFile(filepath.Join(ws.root, "args.json"))
				require.NoError(t, err)
				require.Equal(t, "{}", string(argsContent))

				varsContent, err := os.ReadFile(filepath.Join(ws.root, "variables.json"))
				require.NoError(t, err)
				require.Equal(t, "[]", string(varsContent))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ws := &workspaceStub{root: t.TempDir()}
			ws.code = filepath.Join(ws.root, "main.py")
			require.NoError(t, os.WriteFile(ws.code, []byte("print('hello')\n"), 0o600))

			adapter := New("python3")
			prepared, err := adapter.Prepare(t.Context(), ws, engine.Input{
				Args:      tc.args,
				Variables: tc.variables,
			})
			require.NoError(t, err)
			tc.assert(t, ws, prepared)
		})
	}
}

func TestAdapter_Validate(t *testing.T) {
	testCases := []struct {
		name    string
		binary  string
		wantErr bool
	}{
		{name: "存在的系统命令验证通过", binary: "echo", wantErr: false},
		{name: "不存在的解释器验证报错", binary: "non_existent_python_binary_xyz", wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			adapter := New(tc.binary)
			err := adapter.Validate()
			if tc.wantErr {
				require.Error(t, err)
				require.ErrorContains(t, err, "未找到 Python 解释器")
				return
			}
			require.NoError(t, err)
		})
	}
}
