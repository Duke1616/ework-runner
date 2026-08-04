package runtimefs

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/Duke1616/etask/internal/grpc/scripts/engine"
)

func buildEnvironment(roots engine.ArtifactRoots, workspace string, sandbox bool) []string {
	// 运行时变量覆盖宿主机同名值，保证脚本协议和结果通道稳定。
	overrides := []string{
		"FORCE_COLOR=1",
		"TERM=xterm-256color",
		"PYTHONUNBUFFERED=1",
		"EWORK_RESULT_FD=3",
		"ETASK_WORKSPACE_ROOT=" + workspace,
	}
	if sandbox {
		overrides = append(overrides,
			"HOME="+workspace,
			"TMPDIR="+filepath.Join(workspace, "tmp"),
			"PYTHONDONTWRITEBYTECODE=1",
		)
	}
	if roots.System != "" {
		overrides = append(overrides, "ETASK_SYSTEM_ROOT="+roots.System)
	}
	if roots.Dependencies != "" {
		overrides = append(overrides, "ETASK_DEPENDENCIES_ROOT="+roots.Dependencies)
	}
	// 制品路径放在现有 PYTHONPATH 前，任务应优先使用本次固定版本。
	paths := pythonPaths(roots, workspace)
	if len(paths) > 0 {
		overrides = append(overrides, "PYTHONPATH="+prependPathList(os.Getenv("PYTHONPATH"), paths...))
	}
	return engine.MergeEnvironment(os.Environ(), overrides)
}

func pythonPaths(roots engine.ArtifactRoots, workspace string) []string {
	paths := make([]string, 0, 3)
	if roots.Dependencies != "" {
		paths = append(paths, filepath.Join(roots.Dependencies, "python"), roots.Dependencies)
	}
	if roots.System != "" {
		paths = append(paths, filepath.Join(workspace, ".etask_modules"))
	}
	return paths
}

func prependPathList(current string, paths ...string) string {
	result := make([]string, 0, len(paths)+1)
	for _, path := range paths {
		if path != "" {
			result = append(result, path)
		}
	}
	if current != "" {
		result = append(result, filepath.SplitList(current)...)
	}
	return strings.Join(result, string(os.PathListSeparator))
}
