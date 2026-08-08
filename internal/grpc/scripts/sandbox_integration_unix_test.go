//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package scripts

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Duke1616/etask/internal/grpc/scripts/language/shell"
	"github.com/Duke1616/etask/sdk/executor"
	"github.com/stretchr/testify/require"
)

func TestRuntimeSandboxExecutesTaskAsRequestedIdentity(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("端到端身份降权测试要求 root")
	}
	workspaceParent, err := os.MkdirTemp("/tmp", "etask-sandbox-test-*")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(workspaceParent)) })
	artifactRoot := filepath.Join(workspaceParent, "cache")
	require.NoError(t, os.Mkdir(artifactRoot, 0o750))
	artifactFile := filepath.Join(artifactRoot, "common.sh")
	require.NoError(t, os.WriteFile(artifactFile, []byte("original\n"), 0o444))
	require.NoError(t, os.Chmod(artifactFile, 0o444))

	archiveEnabled := false
	runtime, err := NewRuntime(RuntimeConfig{
		WorkspaceDir: filepath.Join(workspaceParent, "runs"),
		Shell:        shell.Config{Enabled: true, Binary: "/bin/sh"},
		Sandbox: SandboxConfig{
			Mode: SandboxModeRequired, UID: 65534, GID: 65534,
		},
		Archive: ArchiveConfig{Enabled: &archiveEnabled},
	})
	require.NoError(t, err)
	task := executor.NewContext(executor.ContextOptions{
		Context: t.Context(),
		Task: executor.TaskInfo{
			ExecutionID: 1, TaskID: 1, Name: "sandbox", Handler: "shell",
		},
		Params: map[string]string{
			"args": `{}`, "variables": `[]`,
		},
		ExecutionLogger: sandboxExecutionLogger{},
	})
	task.SetProgram(&executor.Program{
		Kind: executor.ProgramKindInline,
		Inline: &executor.InlineProgram{Code: `test "$(id -u):$(id -g)" = "65534:65534"
test "$HOME" = "$ETASK_WORKSPACE_ROOT"
test "$TMPDIR" = "$ETASK_WORKSPACE_ROOT/tmp"
touch "$TMPDIR/child-created"
test "$(cat "$ETASK_SYSTEM_ROOT/common.sh")" = "original"
if printf 'forbidden\n' > "$ETASK_SYSTEM_ROOT/common.sh"; then exit 1; fi
rm "$ETASK_SYSTEM_ROOT/common.sh"
printf 'workspace-only\n' > "$ETASK_SYSTEM_ROOT/common.sh"
		`},
	})
	task.SetArtifactRoots(executor.ArtifactRoots{Default: artifactRoot})
	require.NoError(t, runtime.Handlers()[0].Run(task))
	content, err := os.ReadFile(artifactFile)
	require.NoError(t, err)
	require.Equal(t, "original\n", string(content))
}

type sandboxExecutionLogger struct{}

func (sandboxExecutionLogger) Log(string, ...any) {}
func (sandboxExecutionLogger) Close()             {}
