package runtimefs

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/samber/lo"
)

type directoryInfo struct {
	path    string
	modTime time.Time
	size    int64
}

// PruneDirectory 删除超过保存时间的目录，并按修改时间清理最旧目录直到满足容量限制。
// 根目录中的普通文件不会被删除，避免误伤不属于脚本运行时的数据。
func PruneDirectory(root string, maxAge time.Duration, maxSize int64) error {
	// 首先拒绝危险根目录，避免错误配置触发大范围删除。
	if err := validateRoot(root); err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0o750); err != nil {
		return err
	}

	directories, err := listDirectories(root, maxAge)
	if err != nil {
		return err
	}
	// listDirectories 已按保存时间删除过期目录，此处统计剩余容量。
	total := lo.SumBy(directories, func(d directoryInfo) int64 { return d.size })
	if maxSize <= 0 || total <= maxSize {
		return nil
	}

	// 超出容量时按最久未修改优先回收，直到满足上限。
	sort.Slice(directories, func(i, j int) bool {
		return directories[i].modTime.Before(directories[j].modTime)
	})
	for _, directory := range directories {
		if total <= maxSize {
			break
		}
		if err = os.RemoveAll(directory.path); err != nil {
			return err
		}
		total -= directory.size
	}
	return nil
}

// ValidateDirectory 校验运行时目录不是文件系统根目录，并验证目录可创建且可写。
func ValidateDirectory(path string) error {
	if err := validateRoot(path); err != nil {
		return err
	}
	if err := os.MkdirAll(path, 0o750); err != nil {
		return err
	}
	probe, err := os.CreateTemp(path, ".etask-write-probe-*")
	if err != nil {
		return err
	}
	probePath := probe.Name()
	defer func() {
		_ = probe.Close()
		_ = os.Remove(probePath)
	}()
	if _, err = io.WriteString(probe, "ok"); err != nil {
		return err
	}
	return nil
}

func validateRoot(root string) error {
	clean := filepath.Clean(root)
	volume := filepath.VolumeName(clean)
	if clean == string(os.PathSeparator) || (volume != "" && clean == volume+string(os.PathSeparator)) {
		return fmt.Errorf("拒绝将文件系统根目录作为运行时目录: %s", clean)
	}
	return nil
}

func listDirectories(root string, maxAge time.Duration) ([]directoryInfo, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var deadline time.Time
	if maxAge > 0 {
		deadline = time.Now().Add(-maxAge)
	}
	directories := make([]directoryInfo, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(root, entry.Name())
		info, infoErr := entry.Info()
		if infoErr != nil {
			return nil, infoErr
		}
		if !deadline.IsZero() && info.ModTime().Before(deadline) {
			if err = os.RemoveAll(path); err != nil {
				return nil, err
			}
			continue
		}
		size, sizeErr := directorySize(path)
		if sizeErr != nil {
			return nil, sizeErr
		}
		directories = append(directories, directoryInfo{path: path, modTime: info.ModTime(), size: size})
	}
	return directories, nil
}

func directorySize(root string) (int64, error) {
	var size int64
	err := filepath.WalkDir(root, func(_ string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		size += info.Size()
		return nil
	})
	return size, err
}
