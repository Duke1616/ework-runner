package artifact

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/service/artifact/packer"
	"github.com/klauspost/compress/zstd"
)

// ActiveContents 查询当前执行会使用的全部激活制品及其文件清单。
func (s *service) ActiveContents(ctx context.Context, sourceProjectID int64) ([]domain.ArtifactContent, error) {
	// 复用执行解析规则，确保页面看到的清单与任务真正注入的制品一致。
	refs, err := s.ResolveExecution(ctx, sourceProjectID)
	if err != nil {
		return nil, err
	}
	contents := make([]domain.ArtifactContent, 0, len(refs))
	for _, ref := range refs {
		// 每个清单都从不可变对象读取并校验摘要，不信任数据库中的派生文件列表。
		reader, openErr := s.Open(ctx, ref.ReleaseID, ref.Digest)
		if openErr != nil {
			return nil, openErr
		}
		manifest, readErr := readManifest(reader, ref.Digest)
		closeErr := reader.Close()
		if readErr != nil {
			return nil, fmt.Errorf("读取制品 %d 清单失败: %w", ref.ReleaseID, readErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("关闭制品 %d 数据流失败: %w", ref.ReleaseID, closeErr)
		}
		files := make([]domain.ArtifactManifestFile, 0, len(manifest.Files))
		for _, file := range manifest.Files {
			files = append(files, domain.ArtifactManifestFile{Path: file.Path, Hash: file.Hash, Size: file.Size})
		}
		contents = append(contents, domain.ArtifactContent{
			Release: domain.ArtifactRelease{
				ID: ref.ReleaseID, Scope: ref.Scope, ProjectID: ref.ProjectID,
				Namespace: ref.Namespace, Digest: ref.Digest,
			},
			Files: files,
		})
	}
	return contents, nil
}

// ReadFile 从指定制品发布中读取一个文件的不可变内容。
func (s *service) ReadFile(ctx context.Context, releaseID int64, digest, filePath string) (string, error) {
	// 文件路径先按打包规则校验，避免利用查询接口探测归档外路径。
	clean, err := packer.ValidatePath(filePath)
	if err != nil {
		return "", err
	}
	reader, err := s.Open(ctx, releaseID, digest)
	if err != nil {
		return "", err
	}
	defer reader.Close()

	zstdReader, err := zstd.NewReader(reader, zstd.WithDecoderConcurrency(1))
	if err != nil {
		return "", fmt.Errorf("打开制品压缩数据失败: %w", err)
	}
	defer zstdReader.Close()

	tarReader := tar.NewReader(zstdReader)
	// manifest 必须是首项，先取得文件的预期大小和哈希再读取正文。
	manifest, err := readManifestEntry(tarReader, digest)
	if err != nil {
		return "", err
	}
	var expected *packer.ManifestFile
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
		// 返回内容前再次校验大小和 SHA-256，防止存储对象损坏或被替换。
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

func readManifest(reader io.Reader, expectedDigest string) (packer.Manifest, error) {
	zstdReader, err := zstd.NewReader(reader, zstd.WithDecoderConcurrency(1))
	if err != nil {
		return packer.Manifest{}, fmt.Errorf("打开制品压缩数据失败: %w", err)
	}
	defer zstdReader.Close()

	tarReader := tar.NewReader(zstdReader)
	return readManifestEntry(tarReader, expectedDigest)
}

func readManifestEntry(tarReader *tar.Reader, expectedDigest string) (packer.Manifest, error) {
	// 格式约定 manifest 必须位于 tar 首项，拒绝前置的未知内容。
	header, err := tarReader.Next()
	if err != nil {
		return packer.Manifest{}, fmt.Errorf("读取制品清单失败: %w", err)
	}
	if header.Name != packer.ManifestPath {
		return packer.Manifest{}, fmt.Errorf("制品缺少首项清单文件")
	}
	var manifest packer.Manifest
	if err = json.NewDecoder(tarReader).Decode(&manifest); err != nil {
		return packer.Manifest{}, fmt.Errorf("解析制品清单失败: %w", err)
	}
	if manifest.FormatVersion != packer.FormatVersion || !strings.EqualFold(manifest.Digest, expectedDigest) {
		return packer.Manifest{}, fmt.Errorf("制品清单版本或摘要不匹配")
	}
	// 清空 Digest 后重新序列化，复算发布时的语义内容摘要。
	digest := manifest.Digest
	manifest.Digest = ""
	identity, err := json.Marshal(manifest)
	if err != nil {
		return packer.Manifest{}, fmt.Errorf("校验制品清单失败: %w", err)
	}
	actual := sha256.Sum256(identity)
	if !strings.EqualFold(digest, hex.EncodeToString(actual[:])) {
		return packer.Manifest{}, fmt.Errorf("制品清单内容摘要校验失败")
	}
	manifest.Digest = digest
	return manifest, nil
}
