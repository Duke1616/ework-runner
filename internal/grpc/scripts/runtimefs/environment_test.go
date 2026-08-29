package runtimefs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPrependPathList(t *testing.T) {
	testCases := []struct {
		name     string
		current  string
		paths    []string
		expected string
	}{
		{
			name:     "无现有路径时过滤空值并拼接新路径",
			current:  "",
			paths:    []string{"/opt/python/lib", "", "/usr/local/custom"},
			expected: strings.Join([]string{"/opt/python/lib", "/usr/local/custom"}, string(os.PathListSeparator)),
		},
		{
			name:     "现有路径前置追加新路径并保持原有顺序",
			current:  "/usr/lib/python3.10",
			paths:    []string{"/app/dependencies/python"},
			expected: strings.Join([]string{"/app/dependencies/python", "/usr/lib/python3.10"}, string(os.PathListSeparator)),
		},
		{
			name:     "全部入参均为空返回空字符串",
			current:  "",
			paths:    []string{"", ""},
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := prependPathList(tc.current, tc.paths...)
			require.Equal(t, tc.expected, got)
		})
	}
}

func TestPythonPaths(t *testing.T) {
	testCases := []struct {
		name      string
		roots     mountedArtifactRoots
		workspace string
		assert    func(t *testing.T, paths []string)
	}{
		{
			name: "包含项目、具名依赖与系统依赖的完整路径聚合",
			roots: mountedArtifactRoots{
				project: "/workspace/project",
				named:   "/workspace/dependencies",
				system:  "/workspace/system",
			},
			workspace: "/workspace",
			assert: func(t *testing.T, paths []string) {
				require.Contains(t, paths, "/workspace/project")
				require.Contains(t, paths, filepath.Join("/workspace/dependencies", "python"))
				require.Contains(t, paths, "/workspace/dependencies")
				require.Contains(t, paths, filepath.Join("/workspace", ".etask_modules"))
			},
		},
		{
			name:      "全空依赖时不生成任何 Python 路径",
			roots:     mountedArtifactRoots{},
			workspace: "/workspace",
			assert: func(t *testing.T, paths []string) {
				require.Empty(t, paths)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			paths := pythonPaths(tc.roots, tc.workspace)
			tc.assert(t, paths)
		})
	}
}

func TestBuildEnvironment(t *testing.T) {
	roots := mountedArtifactRoots{
		project: "/workspace/project",
		named:   "/workspace/dependencies",
		system:  "/workspace/system",
	}
	access := NewHostWorkspaceAccess()
	env := buildEnvironment(roots, "/workspace", access)

	require.Contains(t, env, "FORCE_COLOR=1")
	require.Contains(t, env, "TERM=xterm-256color")
	require.Contains(t, env, "PYTHONUNBUFFERED=1")
	require.Contains(t, env, "EWORK_RESULT_FD=3")
	require.Contains(t, env, "ETASK_WORKSPACE_ROOT=/workspace")
	require.Contains(t, env, "ETASK_PROJECT_ROOT=/workspace/project")
	require.Contains(t, env, "ETASK_SYSTEM_ROOT=/workspace/system")
	require.Contains(t, env, "ETASK_DEPENDENCIES_ROOT=/workspace/dependencies")
}
