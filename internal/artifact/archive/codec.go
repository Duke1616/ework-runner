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

//go:generate go tool mockgen -source=./codec.go -package=archivemocks -destination=./mocks/codec.mock.go -typed

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

// IArchiveCodec 定义制品归档与解压协议的编解码契约。
type IArchiveCodec interface {
	// Pack 将代码文件规范化、校验并写入临时压缩包。
	Pack(files []domain.ArtifactFile) (PackedArtifact, error)
	// PackContext 使用调用方提供的内容流生成归档，支持 Blob 文件而不整体加载到内存。
	PackContext(ctx context.Context, files []domain.ArtifactFile, open OpenFile) (PackedArtifact, error)
	// ReadManifest 读取并校验归档首项中的制品清单。
	ReadManifest(reader io.Reader, expectedDigest string) (Manifest, error)
	// ReadFile 读取并校验制品内指定文件的内容。
	ReadFile(reader io.Reader, expectedDigest, filePath string) (string, error)
	// Extract 将制品安全解压到目标目录，并校验清单和全部文件内容。
	Extract(source, target string, expected Metadata, limits ExtractLimits) error
}

// codec 实现 etask 当前使用的 tar.zst 制品归档协议。
type codec struct {
	tempDir string
}

// New 创建使用指定临时目录的 tar.zst 制品编解码器。
func New(tempDir string) IArchiveCodec {
	return &codec{tempDir: tempDir}
}

// Supports 判断当前编解码器是否支持指定格式。
func Supports(format string, version int32) bool {
	return format == Format && version == FormatVersion
}

// Pack 将代码文件规范化、校验并写入临时压缩包。
func (c *codec) Pack(files []domain.ArtifactFile) (PackedArtifact, error) {
	return c.PackContext(context.Background(), files, func(_ context.Context,
		file domain.ArtifactFile) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(file.Code)), nil
	})
}

// PackContext 使用调用方提供的内容流生成归档，支持 Blob 文件而不整体加载到内存。
func (c *codec) PackContext(ctx context.Context, files []domain.ArtifactFile,
	open OpenFile) (PackedArtifact, error) {
	prepared, err := buildManifest(files)
	if err != nil {
		return PackedArtifact{}, err
	}
	return writeArchive(ctx, c.tempDir, prepared, open)
}

// ReadManifest 读取并校验归档首项中的制品清单。
func (c *codec) ReadManifest(reader io.Reader, expectedDigest string) (Manifest, error) {
	return readManifest(reader, expectedDigest)
}

// ReadFile 读取并校验制品内指定文件的内容。
func (c *codec) ReadFile(reader io.Reader, expectedDigest, filePath string) (string, error) {
	return readFile(reader, expectedDigest, filePath)
}
