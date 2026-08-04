package archive

import (
	"archive/tar"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/klauspost/compress/zstd"
)

func readManifest(reader io.Reader, expectedDigest string) (Manifest, error) {
	zstdReader, err := zstd.NewReader(reader, zstd.WithDecoderConcurrency(1))
	if err != nil {
		return Manifest{}, fmt.Errorf("打开制品压缩数据失败: %w", err)
	}
	defer zstdReader.Close()
	return readManifestEntry(tar.NewReader(zstdReader), expectedDigest)
}

func readFile(reader io.Reader, expectedDigest, filePath string) (string, error) {
	clean, err := ValidatePath(filePath)
	if err != nil {
		return "", err
	}
	zstdReader, err := zstd.NewReader(reader, zstd.WithDecoderConcurrency(1))
	if err != nil {
		return "", fmt.Errorf("打开制品压缩数据失败: %w", err)
	}
	defer zstdReader.Close()

	tarReader := tar.NewReader(zstdReader)
	manifest, err := readManifestEntry(tarReader, expectedDigest)
	if err != nil {
		return "", err
	}
	var expected *ManifestFile
	for index := range manifest.Files {
		if manifest.Files[index].Path == clean {
			expected = &manifest.Files[index]
			break
		}
	}
	if expected == nil {
		return "", fmt.Errorf("制品清单中不存在文件: %s", clean)
	}
	for {
		header, nextErr := tarReader.Next()
		if nextErr == io.EOF {
			return "", fmt.Errorf("制品中不存在文件: %s", clean)
		}
		if nextErr != nil {
			return "", fmt.Errorf("读取制品文件失败: %w", nextErr)
		}
		if header.Name != clean {
			continue
		}
		data, readErr := io.ReadAll(tarReader)
		if readErr != nil {
			return "", fmt.Errorf("读取制品文件 %s 失败: %w", clean, readErr)
		}
		actual := sha256.Sum256(data)
		if int64(len(data)) != expected.Size || !strings.EqualFold(hex.EncodeToString(actual[:]), expected.Hash) {
			return "", fmt.Errorf("制品文件 %s 的大小或校验和不匹配", clean)
		}
		return string(data), nil
	}
}

func readManifestEntry(reader *tar.Reader, expectedDigest string) (Manifest, error) {
	header, err := reader.Next()
	if err != nil {
		return Manifest{}, fmt.Errorf("读取制品清单失败: %w", err)
	}
	if header.Name != ManifestPath {
		return Manifest{}, fmt.Errorf("制品缺少首项清单文件")
	}
	var manifest Manifest
	if err = json.NewDecoder(reader).Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("解析制品清单失败: %w", err)
	}
	if manifest.FormatVersion != FormatVersion || !strings.EqualFold(manifest.Digest, expectedDigest) {
		return Manifest{}, fmt.Errorf("制品清单版本或摘要不匹配")
	}
	digest := manifest.Digest
	manifest.Digest = ""
	identity, err := json.Marshal(manifest)
	if err != nil {
		return Manifest{}, fmt.Errorf("校验制品清单失败: %w", err)
	}
	actual := sha256.Sum256(identity)
	if !strings.EqualFold(digest, hex.EncodeToString(actual[:])) {
		return Manifest{}, fmt.Errorf("制品清单内容摘要校验失败")
	}
	manifest.Digest = digest
	return manifest, nil
}
