package runtimefs

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// projectDirectory 创建任务本地目录树，不向脚本暴露缓存路径。
func projectDirectory(source, target string, uid, gid uint32) error {
	resolved, err := filepath.EvalSymlinks(source)
	if err != nil {
		return fmt.Errorf("解析制品路径失败: %w", err)
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return fmt.Errorf("解析制品绝对路径失败: %w", err)
	}
	return projectPath(resolved, target, uid, gid, make(map[string]bool))
}

func projectPath(source, target string, uid, gid uint32, ancestors map[string]bool) error {
	resolved := source
	info, err := os.Lstat(resolved)
	if err != nil {
		return fmt.Errorf("访问制品路径失败: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		resolved, err = filepath.EvalSymlinks(resolved)
		if err != nil {
			return fmt.Errorf("解析制品链接失败: %w", err)
		}
		resolved, err = filepath.Abs(resolved)
		if err != nil {
			return fmt.Errorf("解析制品链接绝对路径失败: %w", err)
		}
		info, err = os.Stat(resolved)
		if err != nil {
			return fmt.Errorf("访问制品链接目标失败: %w", err)
		}
	}
	if info.IsDir() {
		if ancestors[resolved] {
			return fmt.Errorf("制品目录包含循环链接: %s", source)
		}
		ancestors[resolved] = true
		defer delete(ancestors, resolved)
		if err = os.Mkdir(target, 0o750); err != nil {
			return fmt.Errorf("创建投影目录失败: %w", err)
		}
		if err = os.Chown(target, int(uid), int(gid)); err != nil {
			return fmt.Errorf("设置投影目录属主失败: %w", err)
		}
		entries, readErr := os.ReadDir(resolved)
		if readErr != nil {
			return fmt.Errorf("读取制品目录失败: %w", readErr)
		}
		for _, entry := range entries {
			if err = projectPath(filepath.Join(resolved, entry.Name()), filepath.Join(target, entry.Name()), uid, gid, ancestors); err != nil {
				return err
			}
		}
		return nil
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("制品包含不支持的文件类型: %s", source)
	}
	if canHardlinkArtifact(info) {
		if err = os.Link(resolved, target); err == nil {
			return nil
		}
	}
	return copyArtifactFile(resolved, target)
}

func copyArtifactFile(source, target string) (returnErr error) {
	input, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("打开制品文件失败: %w", err)
	}
	defer input.Close()
	output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o444)
	if err != nil {
		return fmt.Errorf("创建投影文件失败: %w", err)
	}
	defer func() {
		if closeErr := output.Close(); returnErr == nil && closeErr != nil {
			returnErr = fmt.Errorf("关闭投影文件失败: %w", closeErr)
		}
		if returnErr != nil {
			_ = os.Remove(target)
		}
	}()
	if _, err = io.Copy(output, input); err != nil {
		return fmt.Errorf("复制制品文件失败: %w", err)
	}
	if err = os.Chmod(target, 0o444); err != nil {
		return fmt.Errorf("设置投影文件权限失败: %w", err)
	}
	return nil
}
