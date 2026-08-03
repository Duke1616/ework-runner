// Package packer 负责将代码文件打包为不可变制品。
package packer

import "github.com/Duke1616/etask/internal/domain"

const (
	// Format 是制品压缩格式。
	Format = "tar.zst"
	// FormatVersion 是制品清单格式版本。
	FormatVersion = int32(1)
	// ManifestPath 是压缩包内清单文件的固定路径。
	ManifestPath = ".etask/manifest.json"
)

// ManifestFile 描述制品中的一个源文件。
type ManifestFile struct {
	Path string `json:"path"`
	Hash string `json:"hash"`
	Size int64  `json:"size"`
}

// Manifest 描述制品的格式版本、内容摘要和文件清单。
type Manifest struct {
	FormatVersion int32          `json:"formatVersion"`
	Digest        string         `json:"digest,omitempty"`
	Files         []ManifestFile `json:"files"`
}

// PackedArtifact 描述已写入临时文件、等待上传的制品。
type PackedArtifact struct {
	Digest       string
	BlobChecksum string
	Path         string
	Size         int64
}

// Packer 定义代码文件打包能力。
type Packer interface {
	// Pack 将代码文件规范化、校验并写入临时压缩包。
	Pack(files []domain.ArtifactFile) (PackedArtifact, error)
}

type tarZstdPacker struct {
	tempDir string
}

// New 创建使用指定临时目录的制品打包器。
func New(tempDir string) Packer {
	return tarZstdPacker{tempDir: tempDir}
}

func (p tarZstdPacker) Pack(files []domain.ArtifactFile) (PackedArtifact, error) {
	prepared, err := buildManifest(files)
	if err != nil {
		return PackedArtifact{}, err
	}
	return writeArchive(p.tempDir, prepared)
}

var _ Packer = tarZstdPacker{}
