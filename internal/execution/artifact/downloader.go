package artifact

// 本文件实现制品流式下载和校验和验证。

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	artifactv1 "github.com/Duke1616/etask/api/proto/gen/etask/artifact/v1"
)

func (c *artifactCache) download(ctx context.Context, client artifactv1.ArtifactServiceClient,
	ref layerRef, file *os.File) error {
	stream, err := client.DownloadArtifact(ctx, &artifactv1.DownloadArtifactRequest{
		ReleaseId: ref.releaseID, Digest: ref.digest,
	})
	if err != nil {
		return fmt.Errorf("请求下载制品失败: %w", err)
	}
	// 下载过程中同步计算压缩对象哈希，不需要再次读取临时文件。
	hash := sha256.New()
	writer := io.MultiWriter(file, hash)
	var written int64
	for {
		chunk, receiveErr := stream.Recv()
		if receiveErr == nil {
			data := chunk.GetData()
			written += int64(len(data))
			// 在写盘前执行声明大小上限，阻止服务端发送无限数据。
			if written > ref.size {
				return fmt.Errorf("制品下载大小超出声明值")
			}
			if _, err = writer.Write(data); err != nil {
				return fmt.Errorf("写入制品临时文件失败: %w", err)
			}
			continue
		}
		if errors.Is(receiveErr, io.EOF) {
			break
		}
		return fmt.Errorf("接收制品数据失败: %w", receiveErr)
	}
	if err = file.Sync(); err != nil {
		return fmt.Errorf("同步制品临时文件失败: %w", err)
	}
	// 完整接收后同时核对字节数与 BlobChecksum，二者任一不符都拒绝解压。
	if written != ref.size {
		return fmt.Errorf("制品大小不一致: 预期=%d 实际=%d", ref.size, written)
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(actual, ref.blobChecksum) {
		return fmt.Errorf("制品校验和不一致: 预期=%s 实际=%s", ref.blobChecksum, actual)
	}
	return nil
}
