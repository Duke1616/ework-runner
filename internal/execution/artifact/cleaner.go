package artifact

// 本文件实现缓存目录清理。

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const artifactTempMaxAge = 24 * time.Hour

type cachedLayer struct {
	path     string
	size     int64
	lastUsed time.Time
}

func pruneCache(root string, maxSize int64) error {
	tempDir := filepath.Join(root, "tmp")
	layersDir := filepath.Join(root, "layers")
	if err := os.MkdirAll(tempDir, 0o750); err != nil {
		return fmt.Errorf("创建制品缓存临时目录失败: %w", err)
	}
	if err := os.MkdirAll(layersDir, 0o750); err != nil {
		return fmt.Errorf("创建制品缓存层目录失败: %w", err)
	}
	// 先清理异常中断遗留的临时文件，再统计可复用缓存层。
	if err := removeStaleTemps(tempDir, time.Now().Add(-artifactTempMaxAge)); err != nil {
		return err
	}
	layers, total, err := inspectLayers(layersDir)
	if err != nil {
		return err
	}
	// 超出容量时按 .ready 的最近使用时间淘汰最旧内容层。
	sort.Slice(layers, func(i, j int) bool { return layers[i].lastUsed.Before(layers[j].lastUsed) })
	for _, layer := range layers {
		if total <= maxSize {
			break
		}
		if err = os.RemoveAll(layer.path); err != nil {
			return fmt.Errorf("清理制品缓存层失败: %w", err)
		}
		total -= layer.size
	}
	return nil
}

func removeStaleTemps(dir string, deadline time.Time) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("读取制品缓存临时目录失败: %w", err)
	}
	for _, entry := range entries {
		info, infoErr := entry.Info()
		if infoErr != nil {
			return fmt.Errorf("读取制品缓存临时文件信息失败: %w", infoErr)
		}
		if info.ModTime().Before(deadline) {
			if removeErr := os.RemoveAll(filepath.Join(dir, entry.Name())); removeErr != nil {
				return fmt.Errorf("清理制品缓存临时文件失败: %w", removeErr)
			}
		}
	}
	return nil
}

func inspectLayers(dir string) ([]cachedLayer, int64, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, 0, fmt.Errorf("读取制品缓存层目录失败: %w", err)
	}
	layers := make([]cachedLayer, 0, len(entries))
	var total int64
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		if !entry.IsDir() {
			// layers 下只允许完整目录，普通文件属于损坏或外部写入。
			if err = os.Remove(path); err != nil {
				return nil, 0, fmt.Errorf("清理非法制品缓存项失败: %w", err)
			}
			continue
		}
		size, sizeErr := directorySize(path)
		if sizeErr != nil {
			return nil, 0, sizeErr
		}
		info, infoErr := os.Stat(filepath.Join(path, ".ready"))
		if infoErr != nil {
			// 没有提交标记的目录不是有效缓存层，启动清理时直接回收。
			if removeErr := os.RemoveAll(path); removeErr != nil {
				return nil, 0, fmt.Errorf("清理未完成制品缓存层失败: %w", removeErr)
			}
			continue
		}
		layers = append(layers, cachedLayer{path: path, size: size, lastUsed: info.ModTime()})
		total += size
	}
	return layers, total, nil
}

func directorySize(root string) (int64, error) {
	var size int64
	err := filepath.WalkDir(root, func(_ string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type().IsRegular() {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			size += info.Size()
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("统计制品缓存层大小失败: %w", err)
	}
	return size, nil
}

func touchArtifact(dir string) {
	now := time.Now()
	_ = os.Chtimes(filepath.Join(dir, ".ready"), now, now)
}
