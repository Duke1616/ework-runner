package packer

import (
	"archive/tar"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/klauspost/compress/zstd"
)

func writeArchive(tempDir string, prepared preparedArtifact) (PackedArtifact, error) {
	if tempDir != "" {
		if err := os.MkdirAll(tempDir, 0o750); err != nil {
			return PackedArtifact{}, fmt.Errorf("创建制品临时目录失败: %w", err)
		}
	}
	tmp, err := os.CreateTemp(tempDir, "etask-artifact-*.tar.zst")
	if err != nil {
		return PackedArtifact{}, fmt.Errorf("创建制品临时文件失败: %w", err)
	}
	tmpPath := tmp.Name()
	success := false
	defer func() {
		_ = tmp.Close()
		if !success {
			_ = os.Remove(tmpPath)
		}
	}()

	// 压缩字节同时写入文件和哈希器，BlobChecksum 校验实际传输对象。
	blobHash := sha256.New()
	zstdWriter, err := zstd.NewWriter(io.MultiWriter(tmp, blobHash), zstd.WithEncoderConcurrency(1))
	if err != nil {
		return PackedArtifact{}, fmt.Errorf("创建 zstd 编码器失败: %w", err)
	}
	tarWriter := tar.NewWriter(zstdWriter)
	if err = writeArchiveEntries(tarWriter, prepared.files, prepared.manifestJSON); err != nil {
		_ = tarWriter.Close()
		_ = zstdWriter.Close()
		return PackedArtifact{}, fmt.Errorf("写入制品失败: %w", err)
	}
	if err = tarWriter.Close(); err != nil {
		_ = zstdWriter.Close()
		return PackedArtifact{}, fmt.Errorf("关闭 tar 制品写入器失败: %w", err)
	}
	if err = zstdWriter.Close(); err != nil {
		return PackedArtifact{}, fmt.Errorf("关闭 zstd 制品写入器失败: %w", err)
	}
	if err = tmp.Sync(); err != nil {
		return PackedArtifact{}, fmt.Errorf("同步制品临时文件失败: %w", err)
	}
	stat, err := tmp.Stat()
	if err != nil {
		return PackedArtifact{}, fmt.Errorf("读取制品临时文件信息失败: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return PackedArtifact{}, fmt.Errorf("关闭制品临时文件失败: %w", err)
	}
	success = true
	return PackedArtifact{
		Digest: prepared.manifest.Digest, BlobChecksum: hex.EncodeToString(blobHash.Sum(nil)),
		Path: tmpPath, Size: stat.Size(),
	}, nil
}

func writeArchiveEntries(writer *tar.Writer, files []domain.ArtifactFile, manifestJSON []byte) error {
	if err := writeTarFile(writer, ManifestPath, manifestJSON); err != nil {
		return err
	}
	for _, file := range files {
		if err := writeTarFile(writer, file.Path, []byte(file.Code)); err != nil {
			return err
		}
	}
	return nil
}

func writeTarFile(writer *tar.Writer, name string, content []byte) error {
	// 清零时间和用户字段，避免机器环境让同一内容产生不同压缩包。
	header := &tar.Header{
		Name: name, Mode: 0o444, Size: int64(len(content)), Typeflag: tar.TypeReg,
		ModTime: time.Unix(0, 0), AccessTime: time.Unix(0, 0), ChangeTime: time.Unix(0, 0),
		Uid: 0, Gid: 0, Uname: "", Gname: "", Format: tar.FormatPAX,
	}
	if err := writer.WriteHeader(header); err != nil {
		return err
	}
	_, err := writer.Write(content)
	return err
}
