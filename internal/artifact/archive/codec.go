// Package archive 实现 etask 制品归档格式的打包、读取和安全解压。
package archive

import (
	"context"
	"io"
	"strings"

	"github.com/Duke1616/etask/internal/domain"
)

// OpenFile 打开一个待归档文件的只读内容流。
type OpenFile func(ctx context.Context, file domain.ArtifactFile) (io.ReadCloser, error)

// PackedArtifact 描述已写入临时文件、等待上传的制品。
type PackedArtifact struct {
	Digest        string
	BlobChecksum  string
	Path          string
	Size          int64
	Format        string
	FormatVersion int32
}

// Metadata 描述待读取或解压制品的格式和语义摘要。
type Metadata struct {
	Digest        string
	Format        string
	FormatVersion int32
}

// ExtractLimits 描述解压制品时允许的资源上限。
type ExtractLimits struct {
	MaxUnpackedSize int64
	MaxFileCount    int
}

// Codec 实现 etask 当前使用的 tar.zst 制品归档协议。
type Codec struct {
	tempDir string
}

// New 创建使用指定临时目录的 tar.zst 制品编解码器。
func New(tempDir string) *Codec {
	return &Codec{tempDir: tempDir}
}

// Supports 判断当前编解码器是否支持指定格式。
func Supports(format string, version int32) bool {
	return format == Format && version == FormatVersion
}

// Pack 将代码文件规范化、校验并写入临时压缩包。
func (c *Codec) Pack(files []domain.ArtifactFile) (PackedArtifact, error) {
	return c.PackContext(context.Background(), files, func(_ context.Context,
		file domain.ArtifactFile) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(file.Code)), nil
	})
}

// PackContext 使用调用方提供的内容流生成归档，支持 Blob 文件而不整体加载到内存。
func (c *Codec) PackContext(ctx context.Context, files []domain.ArtifactFile,
	open OpenFile) (PackedArtifact, error) {
	prepared, err := buildManifest(files)
	if err != nil {
		return PackedArtifact{}, err
	}
	return writeArchive(ctx, c.tempDir, prepared, open)
}

// ReadManifest 读取并校验归档首项中的制品清单。
func (c *Codec) ReadManifest(reader io.Reader, expectedDigest string) (Manifest, error) {
	return readManifest(reader, expectedDigest)
}

// ReadFile 读取并校验制品内指定文件的内容。
func (c *Codec) ReadFile(reader io.Reader, expectedDigest, filePath string) (string, error) {
	return readFile(reader, expectedDigest, filePath)
}
