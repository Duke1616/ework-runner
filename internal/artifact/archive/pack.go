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
	"sort"
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
	files = append([]domain.ArtifactFile(nil), files...)
	// 固定文件顺序是生成稳定 manifest 摘要和压缩内容的前提。
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })

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

	// Digest 只覆盖语义清单且计算时不包含自身，因此同一文件树得到同一摘要。
	identityJSON, err := json.Marshal(manifest)
	if err != nil {
		return preparedArtifact{}, fmt.Errorf("序列化制品清单失败: %w", err)
	}
	digestBytes := sha256.Sum256(identityJSON)
	manifest.Digest = hex.EncodeToString(digestBytes[:])
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return preparedArtifact{}, fmt.Errorf("序列化制品清单失败: %w", err)
	}
	return preparedArtifact{files: files, manifest: manifest, manifestJSON: manifestJSON}, nil
}

func writeArchive(ctx context.Context, tempDir string, prepared preparedArtifact,
	open OpenFile) (PackedArtifact, error) {
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
	header := &tar.Header{
		Name: file.Path, Mode: 0o444, Size: file.Size, Typeflag: tar.TypeReg,
		ModTime: time.Unix(0, 0), AccessTime: time.Unix(0, 0), ChangeTime: time.Unix(0, 0),
		Uid: 0, Gid: 0, Uname: "", Gname: "", Format: tar.FormatPAX,
	}
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
