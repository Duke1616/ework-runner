package runtimefs

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/Duke1616/etask/internal/grpc/scripts/engine"
	"github.com/samber/lo"
)

type mountedArtifactRoots struct {
	project string
	system  string
	named   string
}

func buildEnvironment(roots mountedArtifactRoots, workspace string, access WorkspaceAccess) []string {
	// 运行时变量覆盖宿主机同名值，保证脚本协议和结果通道稳定。
	overrides := []string{
		"FORCE_COLOR=1",
		"TERM=xterm-256color",
		"PYTHONUNBUFFERED=1",
		"EWORK_RESULT_FD=3",
		"ETASK_WORKSPACE_ROOT=" + workspace,
	}
	overrides = append(overrides, access.Environment(workspace)...)
	if roots.project != "" {
		overrides = append(overrides, "ETASK_PROJECT_ROOT="+roots.project)
	}
	if roots.system != "" {
		overrides = append(overrides, "ETASK_SYSTEM_ROOT="+roots.system)
	}
	if roots.named != "" {
		overrides = append(overrides, "ETASK_DEPENDENCIES_ROOT="+roots.named)
	}
	// 制品路径放在现有 PYTHONPATH 前，任务应优先使用本次固定版本。
	paths := pythonPaths(roots, workspace)
	if len(paths) > 0 {
		overrides = append(overrides, "PYTHONPATH="+prependPathList(os.Getenv("PYTHONPATH"), paths...))
	}
	return engine.MergeEnvironment(os.Environ(), overrides)
}

func pythonPaths(roots mountedArtifactRoots, workspace string) []string {
	paths := make([]string, 0, 4)
	if roots.project != "" {
		paths = append(paths, roots.project)
	}
	if roots.named != "" {
		paths = append(paths, filepath.Join(roots.named, "python"), roots.named)
	}
	if roots.system != "" {
		paths = append(paths, filepath.Join(workspace, ".etask_modules"))
	}
	return paths
}

func prependPathList(current string, paths ...string) string {
	result := lo.Filter(paths, func(path string, _ int) bool {
		return path != ""
	})
	if current != "" {
		result = append(result, filepath.SplitList(current)...)
	}
	return strings.Join(result, string(os.PathListSeparator))
}
