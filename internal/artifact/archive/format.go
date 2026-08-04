package archive

import (
	"fmt"
	"path"
	"strings"
)

const (
	// Format 是当前制品压缩格式。
	Format = "tar.zst"
	// FormatVersion 是当前制品清单格式版本。
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

// ValidatePath 校验并规范化制品内的相对路径。
func ValidatePath(name string) (string, error) {
	if name == "" || name != strings.TrimSpace(name) || strings.HasPrefix(name, "/") ||
		strings.ContainsAny(name, "\\\x00") {
		return "", fmt.Errorf("非法的制品路径: %q", name)
	}
	clean := path.Clean(name)
	if clean != name {
		return "", fmt.Errorf("制品路径必须是规范相对路径: %q", name)
	}
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("制品路径超出根目录: %q", name)
	}
	if clean == ".etask" || strings.HasPrefix(clean, ".etask/") {
		return "", fmt.Errorf("制品路径使用了保留目录: %q", name)
	}
	return clean, nil
}
