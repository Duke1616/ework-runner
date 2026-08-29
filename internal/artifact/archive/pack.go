package archive

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/klauspost/compress/zstd"
)

type preparedArtifact struct {
	files        []domain.ArtifactFile
	manifest     Manifest
	manifestJSON []byte
}

func buildManifest(files []domain.ArtifactFile) (preparedArtifact, error) {
	if len(files) == 0 {
		return preparedArtifact{}, fmt.Errorf("代码资源中没有可发布的文件")
	}
	// 拷贝切片避免污染外部入参；按文件路径严格字典序排序，保证每次生成的 Manifest 摘要与压缩包字节流完全确定，从而最大化命中缓存。
	files = slices.Clone(files)
	slices.SortFunc(files, func(a, b domain.ArtifactFile) int {
		return strings.Compare(a.Path, b.Path)
	})

	manifest := Manifest{FormatVersion: FormatVersion, Files: make([]ManifestFile, 0, len(files))}
	seen := make(map[string]struct{}, len(files))
	for i := range files {
		clean, err := ValidatePath(files[i].Path)
		if err != nil {
			return preparedArtifact{}, err
		}
		if _, ok := seen[clean]; ok {
			return preparedArtifact{}, fmt.Errorf("制品中存在重复路径: %s", clean)
		}
		seen[clean] = struct{}{}
		files[i].Path = clean
		storage := files[i].StorageType
		if storage == "" {
			storage = domain.CodebookContentInline
		}
		if !storage.Valid() {
			return preparedArtifact{}, fmt.Errorf("制品源文件 %s 的内容存储类型非法", clean)
		}
		files[i].StorageType = storage
		if storage == domain.CodebookContentInline {
			actualHash := sha256.Sum256([]byte(files[i].Code))
			actualHashString := hex.EncodeToString(actualHash[:])
			if files[i].Hash != "" && !strings.EqualFold(files[i].Hash, actualHashString) {
				return preparedArtifact{}, fmt.Errorf("制品源文件 %s 的源码校验和与版本记录不一致", clean)
			}
			files[i].Hash = actualHashString
			files[i].Size = int64(len(files[i].Code))
		} else {
			if strings.TrimSpace(files[i].ObjectKey) == "" || files[i].Size < 0 || !validSHA256(files[i].Hash) {
				return preparedArtifact{}, fmt.Errorf("制品源文件 %s 的 Blob 元数据不完整", clean)
			}
		}
		manifest.Files = append(manifest.Files, ManifestFile{
			Path: clean, Hash: strings.ToLower(files[i].Hash), Size: files[i].Size,
		})
	}

	digest, err := manifest.CalculateDigest()
	if err != nil {
		return preparedArtifact{}, err
	}
	manifest.Digest = digest
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return preparedArtifact{}, fmt.Errorf("序列化制品清单失败: %w", err)
	}
	return preparedArtifact{files: files, manifest: manifest, manifestJSON: manifestJSON}, nil
}

func writeArchive(ctx context.Context, tempDir string, prepared preparedArtifact,
	open OpenFile) (PackedArtifact, error) {
	if tempDir != "" {
		if err := os.MkdirAll(tempDir, PermDir); err != nil {
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

	blobHash := sha256.New()
	zstdWriter, err := zstd.NewWriter(io.MultiWriter(tmp, blobHash), zstd.WithEncoderConcurrency(1))
	if err != nil {
		return PackedArtifact{}, fmt.Errorf("创建 zstd 编码器失败: %w", err)
	}
	tarWriter := tar.NewWriter(zstdWriter)
	if err = writeArchiveEntries(ctx, tarWriter, prepared.files, prepared.manifestJSON, open); err != nil {
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
		Path: tmpPath, Size: stat.Size(), Format: Format, FormatVersion: FormatVersion,
	}, nil
}

func writeArchiveEntries(ctx context.Context, writer *tar.Writer, files []domain.ArtifactFile,
	manifestJSON []byte, open OpenFile) error {
	if err := writeTarFile(writer, ManifestPath, manifestJSON); err != nil {
		return err
	}
	for _, file := range files {
		if err := writeTarStream(ctx, writer, file, open); err != nil {
			return err
		}
	}
	return nil
}

func writeTarStream(ctx context.Context, writer *tar.Writer, file domain.ArtifactFile,
	open OpenFile) (returnErr error) {
	if open == nil {
		return fmt.Errorf("制品源文件 %s 缺少内容读取器", file.Path)
	}
	header := newDeterministicTarHeader(file.Path, file.Size)
	if err := writer.WriteHeader(header); err != nil {
		return err
	}
	reader, err := open(ctx, file)
	if err != nil {
		return fmt.Errorf("打开制品源文件 %s 失败: %w", file.Path, err)
	}
	defer func() {
		if closeErr := reader.Close(); returnErr == nil && closeErr != nil {
			returnErr = fmt.Errorf("关闭制品源文件 %s 失败: %w", file.Path, closeErr)
		}
	}()
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(writer, hash), io.LimitReader(reader, file.Size+1))
	if err != nil {
		return fmt.Errorf("写入制品源文件 %s 失败: %w", file.Path, err)
	}
	if written != file.Size {
		return fmt.Errorf("制品源文件 %s 大小不一致: 预期=%d 实际=%d", file.Path, file.Size, written)
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(actual, file.Hash) {
		return fmt.Errorf("制品源文件 %s 校验和不一致", file.Path)
	}
	return nil
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func writeTarFile(writer *tar.Writer, name string, content []byte) error {
	header := newDeterministicTarHeader(name, int64(len(content)))
	if err := writer.WriteHeader(header); err != nil {
		return err
	}
	_, err := writer.Write(content)
	return err
}

// newDeterministicTarHeader 创建确定性归档的 tar 文件头（清零时间戳与系统用户元数据，固定 PAX 格式和只读权限）。
func newDeterministicTarHeader(name string, size int64) *tar.Header {
	return &tar.Header{
		Name: name, Mode: int64(PermReadOnly), Size: size, Typeflag: tar.TypeReg,
		ModTime: time.Unix(0, 0), AccessTime: time.Unix(0, 0), ChangeTime: time.Unix(0, 0),
		Uid: 0, Gid: 0, Uname: "", Gname: "", Format: tar.FormatPAX,
	}
}
