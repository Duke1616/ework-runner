package archive

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/zstd"
)

// Extract 将制品安全解压到目标目录，并校验清单和全部文件内容。
func (c *Codec) Extract(source, target string, expected Metadata, limits ExtractLimits) error {
	if !Supports(expected.Format, expected.FormatVersion) {
		return fmt.Errorf("不支持的制品格式: %s/%d", expected.Format, expected.FormatVersion)
	}
	if limits.MaxUnpackedSize <= 0 || limits.MaxFileCount <= 0 {
		return fmt.Errorf("制品解压限制非法")
	}
	if err := extractArchive(source, target, limits); err != nil {
		return err
	}
	return validateExtractedArtifact(target, expected)
}

func extractArchive(source, target string, limits ExtractLimits) error {
	file, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("打开制品压缩包失败: %w", err)
	}
	defer file.Close()

	reader, err := zstd.NewReader(file)
	if err != nil {
		return fmt.Errorf("打开制品 zstd 数据失败: %w", err)
	}
	defer reader.Close()

	tarReader := tar.NewReader(reader)
	var totalSize int64
	fileCount := 0
	for {
		header, readErr := tarReader.Next()
		if errors.Is(readErr, io.EOF) {
			return nil
		}
		if readErr != nil {
			return fmt.Errorf("读取制品 tar 数据失败: %w", readErr)
		}
		fileCount++
		if fileCount > limits.MaxFileCount {
			return fmt.Errorf("制品文件数量超出限制")
		}
		if header.Size < 0 || header.Size > limits.MaxUnpackedSize-totalSize {
			return fmt.Errorf("制品解压大小超出限制")
		}
		totalSize += header.Size

		path, err := safeExtractPath(target, header.Name)
		if err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err = os.MkdirAll(path, 0o750); err != nil {
				return fmt.Errorf("创建制品目录失败: %w", err)
			}
		case tar.TypeReg, tar.TypeRegA:
			if err = extractFile(tarReader, path, header.Size); err != nil {
				return err
			}
		default:
			return fmt.Errorf("制品包含不支持的文件类型: %s", header.Name)
		}
	}
}

func extractFile(reader io.Reader, path string, size int64) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("创建制品父目录失败: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o444)
	if err != nil {
		return fmt.Errorf("创建制品文件失败: %w", err)
	}
	_, copyErr := io.CopyN(file, reader, size)
	closeErr := file.Close()
	if copyErr != nil {
		return fmt.Errorf("写入制品文件失败: %w", copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("关闭制品文件失败: %w", closeErr)
	}
	if err = os.Chmod(path, 0o444); err != nil {
		return fmt.Errorf("设置制品文件只读权限失败: %w", err)
	}
	return nil
}

func safeExtractPath(root, name string) (string, error) {
	name = filepath.FromSlash(name)
	if name == "" || filepath.IsAbs(name) {
		return "", fmt.Errorf("制品包含非法路径: %q", name)
	}
	clean := filepath.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("制品路径超出缓存目录: %q", name)
	}
	resolved := filepath.Join(root, clean)
	relative, err := filepath.Rel(root, resolved)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("制品路径超出缓存目录: %q", name)
	}
	return resolved, nil
}
