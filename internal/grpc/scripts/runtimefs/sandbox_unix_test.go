//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package runtimefs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Duke1616/etask/internal/grpc/scripts/engine"
	"github.com/stretchr/testify/require"
)

func TestSandboxWorkspaceProjectsArtifactsWithoutCacheMutation(t *testing.T) {
	source := t.TempDir()
	sourceFile := filepath.Join(source, "common.sh")
	require.NoError(t, os.WriteFile(sourceFile, []byte("original\n"), 0o444))
	require.NoError(t, os.Chmod(sourceFile, 0o444))

	workspaceParent := t.TempDir()
	factory, err := NewWorkspaceFactory(
		WorkspaceConfig{Dir: filepath.Join(workspaceParent, "runs")}, isolatedTestAccess(t),
	)
	require.NoError(t, err)
	workspace, err := factory.Create(engine.WorkspaceOptions{
		ExecutionID: 1, Extension: ".sh", Code: []byte("echo ok\n"),
		Artifacts: engine.ArtifactRoots{System: source},
	})
	require.NoError(t, err)
	defer func() { require.NoError(t, workspace.Close()) }()

	projectedRoot := filepath.Join(workspace.Root(), "system")
	info, err := os.Lstat(projectedRoot)
	require.NoError(t, err)
	require.NotEqual(t, os.ModeSymlink, info.Mode()&os.ModeSymlink)
	projectedFile := filepath.Join(projectedRoot, "common.sh")
	require.FileExists(t, projectedFile)
	require.NoError(t, os.Remove(projectedFile))
	require.NoError(t, os.WriteFile(projectedFile, []byte("replacement\n"), 0o600))

	content, err := os.ReadFile(sourceFile)
	require.NoError(t, err)
	require.Equal(t, "original\n", string(content))
	requireEnvironment(t, workspace.Environment(), "HOME", workspace.Root())
	requireEnvironment(t, workspace.Environment(), "TMPDIR", filepath.Join(workspace.Root(), "tmp"))
	requireEnvironment(t, workspace.Environment(), "PYTHONDONTWRITEBYTECODE", "1")
	require.DirExists(t, filepath.Join(workspace.Root(), "tmp"))
}

func TestSandboxWorkspacePreservesNamedArtifactLayout(t *testing.T) {
	factory, err := NewWorkspaceFactory(
		WorkspaceConfig{Dir: filepath.Join(t.TempDir(), "runs")}, isolatedTestAccess(t),
	)
	require.NoError(t, err)
	workspace, err := factory.Create(engine.WorkspaceOptions{
		ExecutionID: 1, Extension: ".py", Code: []byte("print('ok')\n"),
		Artifacts: engine.ArtifactRoots{Named: map[string]string{
			"ops_common": createPythonArtifact(t, "named"),
		}},
	})
	require.NoError(t, err)
	defer func() { require.NoError(t, workspace.Close()) }()
	require.FileExists(t, filepath.Join(
		workspace.Root(), "dependencies", "python", "ops_common", "private", "util.py",
	))
}

func TestSandboxWorkspaceRejectsArtifactSymlinkCycle(t *testing.T) {
	source := t.TempDir()
	require.NoError(t, os.Symlink(source, filepath.Join(source, "cycle")))
	factory, err := NewWorkspaceFactory(
		WorkspaceConfig{Dir: filepath.Join(t.TempDir(), "runs")}, isolatedTestAccess(t),
	)
	require.NoError(t, err)
	_, err = factory.Create(engine.WorkspaceOptions{
		ExecutionID: 1, Extension: ".sh", Code: []byte("echo ok\n"),
		Artifacts: engine.ArtifactRoots{System: source},
	})
	require.ErrorContains(t, err, "循环链接")
}

func isolatedTestAccess(t *testing.T) WorkspaceAccess {
	t.Helper()
	uid, gid := uint32(os.Geteuid()), uint32(os.Getegid())
	if uid == 0 {
		uid, gid = 65534, 65534
	}
	access, err := NewIsolatedWorkspaceAccess(uid, gid)
	require.NoError(t, err)
	return access
}
