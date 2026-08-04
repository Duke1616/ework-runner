package runtimefs

import (
	"fmt"
	"os"
	"path/filepath"
)

// WorkspaceAccess 定义脚本身份访问工作区和制品的完整策略。
type WorkspaceAccess interface {
	PrepareRoot(path string) error
	PrepareWorkspace(path string) error
	Own(path string) error
	MountArtifact(source, target string) error
	Environment(workspace string) []string
}

type hostWorkspaceAccess struct{}

// NewHostWorkspaceAccess 创建沿用 Executor 身份和符号链接挂载的可信工作区策略。
func NewHostWorkspaceAccess() WorkspaceAccess {
	return hostWorkspaceAccess{}
}

func (hostWorkspaceAccess) PrepareRoot(string) error      { return nil }
func (hostWorkspaceAccess) PrepareWorkspace(string) error { return nil }
func (hostWorkspaceAccess) Own(string) error              { return nil }
func (hostWorkspaceAccess) Environment(string) []string   { return nil }
func (hostWorkspaceAccess) MountArtifact(source, target string) error {
	return os.Symlink(source, target)
}

type isolatedWorkspaceAccess struct {
	uid uint32
	gid uint32
}

// NewIsolatedWorkspaceAccess 创建降权身份使用的只读制品投影策略。
func NewIsolatedWorkspaceAccess(uid, gid uint32) (WorkspaceAccess, error) {
	if uid == 0 || gid == 0 {
		return nil, fmt.Errorf("隔离工作区 UID/GID 必须是非 root 身份")
	}
	return isolatedWorkspaceAccess{uid: uid, gid: gid}, nil
}

func (a isolatedWorkspaceAccess) PrepareRoot(root string) error {
	parent := filepath.Dir(root)
	parentInfo, err := os.Stat(parent)
	if err != nil {
		return err
	}
	// 仅在父目录没有 other execute 时授予脚本组穿越权限，不开放目录枚举。
	if parentInfo.Mode().Perm()&0o001 == 0 {
		if err = os.Chown(parent, -1, int(a.gid)); err != nil {
			return err
		}
		parentMode := (parentInfo.Mode().Perm() &^ 0o070) | 0o010
		if err = os.Chmod(parent, parentMode); err != nil {
			return err
		}
	}
	if err = os.Chown(root, -1, int(a.gid)); err != nil {
		return err
	}
	return os.Chmod(root, 0o710)
}

func (a isolatedWorkspaceAccess) PrepareWorkspace(root string) error {
	if err := a.Own(root); err != nil {
		return err
	}
	temporary := filepath.Join(root, "tmp")
	if err := os.Mkdir(temporary, 0o700); err != nil {
		return err
	}
	return a.Own(temporary)
}

func (a isolatedWorkspaceAccess) Own(path string) error {
	return os.Chown(path, int(a.uid), int(a.gid))
}

func (a isolatedWorkspaceAccess) MountArtifact(source, target string) error {
	return projectDirectory(source, target, a.uid, a.gid)
}

func (isolatedWorkspaceAccess) Environment(workspace string) []string {
	return []string{
		"HOME=" + workspace,
		"TMPDIR=" + filepath.Join(workspace, "tmp"),
		"PYTHONDONTWRITEBYTECODE=1",
	}
}
