package runtimefs

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Duke1616/etask/internal/grpc/scripts/engine"
)

type WorkspaceFactory struct {
	config WorkspaceConfig
}

// NewWorkspaceFactory 创建文件系统工作区工厂。
func NewWorkspaceFactory(config WorkspaceConfig) (*WorkspaceFactory, error) {
	normalized, err := NormalizeWorkspaceConfig(config)
	if err != nil {
		return nil, err
	}
	return &WorkspaceFactory{config: normalized}, nil
}

// Create 创建代码文件并挂载制品目录。
func (f *WorkspaceFactory) Create(options engine.WorkspaceOptions) (engine.Workspace, error) {
	if err := os.MkdirAll(f.config.Dir, 0o750); err != nil {
		return nil, fmt.Errorf("创建任务工作区根目录失败: %w", err)
	}
	if err := prepareWorkspaceRoot(f.config.Dir, f.config.Sandbox); err != nil {
		return nil, fmt.Errorf("准备任务工作区根目录失败: %w", err)
	}
	root, err := os.MkdirTemp(f.config.Dir, fmt.Sprintf("%d-*", options.ExecutionID))
	if err != nil {
		return nil, fmt.Errorf("创建任务工作区失败: %w", err)
	}
	ws := &workspace{root: root, sandbox: f.config.Sandbox}
	if err = ws.setTaskOwner(root); err != nil {
		_ = ws.Close()
		return nil, fmt.Errorf("设置任务工作区属主失败: %w", err)
	}
	// prepare 任一步失败都删除本次临时目录，避免留下不可识别的半成品。
	if err = ws.prepare(options); err != nil {
		_ = ws.Close()
		return nil, err
	}
	return ws, nil
}

func prepareWorkspaceRoot(root string, sandbox engine.Sandbox) error {
	if !sandbox.Enabled {
		return nil
	}
	parent := filepath.Dir(root)
	parentInfo, err := os.Stat(parent)
	if err != nil {
		return err
	}
	// 仅在父目录没有 other execute 时授予沙箱组穿越权限，不开放目录枚举。
	if parentInfo.Mode().Perm()&0o001 == 0 {
		if err = os.Chown(parent, -1, int(sandbox.GID)); err != nil {
			return err
		}
		parentMode := (parentInfo.Mode().Perm() &^ 0o070) | 0o010
		if err = os.Chmod(parent, parentMode); err != nil {
			return err
		}
	}
	if err = os.Chown(root, -1, int(sandbox.GID)); err != nil {
		return err
	}
	return os.Chmod(root, 0o710)
}

// Prune 清理过期工作区。
func (f *WorkspaceFactory) Prune() error {
	return PruneDirectory(f.config.Dir, f.config.MaxAge, 0)
}

// Validate 校验工作区目录可写。
func (f *WorkspaceFactory) Validate() error {
	if err := os.MkdirAll(f.config.Dir, 0o750); err != nil {
		return err
	}
	if err := prepareWorkspaceRoot(f.config.Dir, f.config.Sandbox); err != nil {
		return err
	}
	return ValidateDirectory(f.config.Dir)
}

type workspace struct {
	root        string
	codeFile    string
	artifacts   engine.ArtifactRoots
	environment []string
	sandbox     engine.Sandbox
}

func (w *workspace) prepare(options engine.WorkspaceOptions) error {
	codeName := "task" + options.Extension
	// SYSTEM 层固定挂载，并额外映射为 etask Python 命名空间。
	if options.Artifacts.System != "" {
		mounted, err := w.mount("system", options.Artifacts.System)
		if err != nil {
			return err
		}
		w.artifacts.System = mounted
		modules := filepath.Join(w.root, ".etask_modules")
		if err = w.mkdirTask(modules, 0o750); err != nil {
			return fmt.Errorf("创建 Python 制品命名空间失败: %w", err)
		}
		// 显式 python 目录用于纯 Python 制品；混合语言 SYSTEM 制品则将根目录映射到 etask。
		pythonRoot := mounted
		pythonDir := filepath.Join(mounted, "python")
		if info, statErr := os.Stat(pythonDir); statErr == nil && info.IsDir() {
			pythonRoot = pythonDir
		} else if statErr != nil && !os.IsNotExist(statErr) {
			return fmt.Errorf("检查 SYSTEM Python 目录失败: %w", statErr)
		}
		if err = os.Symlink(pythonRoot, filepath.Join(modules, "etask")); err != nil {
			return fmt.Errorf("挂载 SYSTEM Python 命名空间失败: %w", err)
		}
	}
	// 新制品运行时直接提供不可变具名层，由工作区负责组合最终目录。
	if len(options.Artifacts.Named) > 0 {
		mounted, err := w.mountNamedDependencies(options.Artifacts.Named)
		if err != nil {
			return err
		}
		w.artifacts.Dependencies = mounted
	} else if options.Artifacts.Dependencies != "" {
		// 保留旧 Preparer 已聚合依赖目录的兼容路径。
		mounted, err := w.mount("dependencies", options.Artifacts.Dependencies)
		if err != nil {
			return err
		}
		w.artifacts.Dependencies = mounted
	}
	// 脚本和环境最后生成，确保 Handler 看到的是完整且稳定的工作区。
	w.codeFile = filepath.Join(w.root, codeName)
	if err := os.WriteFile(w.codeFile, options.Code, 0o700); err != nil {
		return fmt.Errorf("写入任务脚本失败: %w", err)
	}
	if err := w.setTaskOwner(w.codeFile); err != nil {
		return fmt.Errorf("设置任务脚本属主失败: %w", err)
	}
	if w.sandbox.Enabled {
		if err := w.mkdirTask(filepath.Join(w.root, "tmp"), 0o700); err != nil {
			return fmt.Errorf("创建任务临时目录失败: %w", err)
		}
	}
	w.environment = buildEnvironment(w.artifacts, w.root, w.sandbox.Enabled)
	return nil
}

func (w *workspace) mount(name, source string) (string, error) {
	target := filepath.Join(w.root, name)
	return w.mountDirectory(name, source, target)
}

func (w *workspace) mountNamedDependencies(layers map[string]string) (string, error) {
	root := filepath.Join(w.root, "dependencies")
	pythonRoot := filepath.Join(root, "python")
	if err := w.mkdirTask(pythonRoot, 0o750); err != nil {
		return "", fmt.Errorf("创建制品依赖目录失败: %w", err)
	}
	names := make([]string, 0, len(layers))
	for name := range layers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := validateDependencyName(name); err != nil {
			return "", err
		}
		mounted, err := w.mountDirectory(name, layers[name], filepath.Join(root, name))
		if err != nil {
			return "", err
		}
		pythonDir := filepath.Join(mounted, "python")
		info, statErr := os.Stat(pythonDir)
		if os.IsNotExist(statErr) {
			continue
		}
		if statErr != nil {
			return "", fmt.Errorf("检查制品依赖 %s 的 Python 目录失败: %w", name, statErr)
		}
		if info.IsDir() {
			if err = os.Symlink(pythonDir, filepath.Join(pythonRoot, name)); err != nil {
				return "", fmt.Errorf("挂载制品依赖 %s 的 Python 命名空间失败: %w", name, err)
			}
		}
	}
	return root, nil
}

func (w *workspace) mountDirectory(name, source, target string) (string, error) {
	absolute, err := filepath.Abs(source)
	if err != nil {
		return "", fmt.Errorf("解析 %s 制品目录失败: %w", name, err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", fmt.Errorf("访问 %s 制品目录失败: %w", name, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s 制品路径不是目录: %s", name, absolute)
	}
	if w.sandbox.Enabled {
		if err = projectDirectory(absolute, target, w.sandbox); err != nil {
			return "", fmt.Errorf("投影 %s 制品失败: %w", name, err)
		}
	} else if err = os.Symlink(absolute, target); err != nil {
		return "", fmt.Errorf("挂载 %s 制品失败: %w", name, err)
	}
	return target, nil
}

func validateDependencyName(name string) error {
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name || name == "etask" {
		return fmt.Errorf("制品挂载名称非法或使用了运行时保留名: %q", name)
	}
	return nil
}

func (w *workspace) Root() string {
	return w.root
}

func (w *workspace) CodeFile() string {
	return w.codeFile
}

func (w *workspace) Environment() []string {
	return w.environment
}

func (w *workspace) WriteFile(name string, content []byte, mode os.FileMode) (string, error) {
	if filepath.Base(name) != name {
		return "", fmt.Errorf("工作区文件名非法: %s", name)
	}
	path := filepath.Join(w.root, name)
	if err := os.WriteFile(path, content, mode); err != nil {
		return "", err
	}
	if err := w.setTaskOwner(path); err != nil {
		return "", err
	}
	return path, nil
}

func (w *workspace) mkdirTask(path string, mode os.FileMode) error {
	if err := os.MkdirAll(path, mode); err != nil {
		return err
	}
	if !w.sandbox.Enabled {
		return nil
	}
	relative, err := filepath.Rel(w.root, path)
	if err != nil || relative == ".." || filepath.IsAbs(relative) {
		return fmt.Errorf("任务目录超出工作区: %s", path)
	}
	current := w.root
	if err = w.setTaskOwner(current); err != nil {
		return err
	}
	if relative == "." {
		return nil
	}
	for _, part := range strings.Split(relative, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		if err = w.setTaskOwner(current); err != nil {
			return err
		}
	}
	return nil
}

func (w *workspace) setTaskOwner(path string) error {
	if !w.sandbox.Enabled {
		return nil
	}
	return os.Chown(path, int(w.sandbox.UID), int(w.sandbox.GID))
}

func (w *workspace) Close() error {
	return os.RemoveAll(w.root)
}

var _ engine.WorkspaceFactory = (*WorkspaceFactory)(nil)
var _ engine.Workspace = (*workspace)(nil)
