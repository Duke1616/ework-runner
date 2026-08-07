package artifact

// 本文件实现制品流式下载和校验和验证。

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"

	executorartifact "github.com/Duke1616/etask/sdk/executor/artifact"
)

func (c *artifactCache) download(ctx context.Context, downloader executorartifact.Downloader,
	ref layerRef, file *os.File) error {
	hash := sha256.New()
	writer := &boundedWriter{target: io.MultiWriter(file, hash), max: ref.size}
	var err error
	switch ref.kind {
	case layerProjectSource:
		err = downloader.DownloadSource(ctx, ref.sourceRef(), writer)
	case layerArtifact:
		err = downloader.DownloadArtifact(ctx, ref.artifactRef(), writer)
	default:
		err = fmt.Errorf("未知制品层类型")
	}
	if err != nil {
		return fmt.Errorf("下载制品失败: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("同步制品临时文件失败: %w", err)
	}
	// 完整接收后同时核对字节数与 BlobChecksum，二者任一不符都拒绝解压。
	if writer.written != ref.size {
		return fmt.Errorf("制品大小不一致: 预期=%d 实际=%d", ref.size, writer.written)
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(actual, ref.blobChecksum) {
		return fmt.Errorf("制品校验和不一致: 预期=%s 实际=%s", ref.blobChecksum, actual)
	}
	return nil
}

type boundedWriter struct {
	target  io.Writer
	max     int64
	written int64
}

func (w *boundedWriter) Write(data []byte) (int, error) {
	if w.written+int64(len(data)) > w.max {
		return 0, fmt.Errorf("制品下载大小超出声明值")
	}
	n, err := w.target.Write(data)
	w.written += int64(n)
	if err != nil {
		return n, fmt.Errorf("写入制品临时文件失败: %w", err)
	}
	return n, nil
}
