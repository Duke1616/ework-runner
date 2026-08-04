package artifact

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	artifactv1 "github.com/Duke1616/etask/api/proto/gen/etask/artifact/v1"
	artifactarchive "github.com/Duke1616/etask/internal/artifact/archive"
)

// materialize 在临时目录完成下载、解压和校验，最后原子提交缓存层。
func (c *artifactCache) materialize(ctx context.Context, client artifactv1.ArtifactServiceClient,
	ref layerRef, targetDir string) (string, error) {
	part, err := os.CreateTemp(c.layout.tempDir(), ref.digest+"-*.part")
	if err != nil {
		return "", fmt.Errorf("创建制品临时文件失败: %w", err)
	}
	partPath := part.Name()
	defer func() {
		_ = part.Close()
		_ = os.Remove(partPath)
	}()
	if err = c.download(ctx, client, ref, part); err != nil {
		return "", err
	}
	if err = part.Close(); err != nil {
		return "", fmt.Errorf("关闭制品临时文件失败: %w", err)
	}

	extractDir, err := os.MkdirTemp(c.layout.tempDir(), ref.digest+"-*.extract")
	if err != nil {
		return "", fmt.Errorf("创建制品解压目录失败: %w", err)
	}
	defer os.RemoveAll(extractDir)
	if err = c.codec.Extract(partPath, extractDir, ref.metadata(), artifactarchive.ExtractLimits{
		MaxUnpackedSize: c.cfg.MaxUnpackedSize, MaxFileCount: c.cfg.MaxFileCount,
	}); err != nil {
		return "", err
	}
	if err = writeCacheMarker(extractDir, ref.marker()); err != nil {
		return "", err
	}
	return commitLayer(extractDir, targetDir, ref)
}

func writeCacheMarker(dir string, marker cacheMarker) error {
	data, err := json.Marshal(marker)
	if err != nil {
		return fmt.Errorf("序列化制品缓存标记失败: %w", err)
	}
	if err = os.WriteFile(filepath.Join(dir, ".ready"), data, 0o440); err != nil {
		return fmt.Errorf("写入制品缓存标记失败: %w", err)
	}
	return nil
}

func commitLayer(sourceDir, targetDir string, ref layerRef) (string, error) {
	if err := os.Rename(sourceDir, targetDir); err == nil {
		return targetDir, nil
	}
	if readyArtifact(targetDir, ref) {
		return targetDir, nil
	}
	if err := os.RemoveAll(targetDir); err != nil {
		return "", fmt.Errorf("清理无效制品缓存失败: %w", err)
	}
	if err := os.Rename(sourceDir, targetDir); err != nil {
		if readyArtifact(targetDir, ref) {
			return targetDir, nil
		}
		return "", fmt.Errorf("提交制品缓存失败: %w", err)
	}
	return targetDir, nil
}
