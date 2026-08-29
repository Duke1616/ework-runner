package archive

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
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

	// PermDir 是制品解压与缓存目录的标准访问权限（rwxr-x---）。
	PermDir os.FileMode = 0o750
	// PermReadOnly 是物化文件的只读保护权限（r--r--r--），防止执行时被意外篡改。
	PermReadOnly os.FileMode = 0o444
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

// CalculateDigest 计算当前清单的确定性内容摘要（计算时不包含自身 Digest 字段）。
func (m Manifest) CalculateDigest() (string, error) {
	copyManifest := m
	copyManifest.Digest = ""
	identity, err := json.Marshal(copyManifest)
	if err != nil {
		return "", fmt.Errorf("序列化制品清单失败: %w", err)
	}
	sum := sha256.Sum256(identity)
	return hex.EncodeToString(sum[:]), nil
}

// VerifyDigest 校验清单的格式版本、预期摘要以及自身内容摘要的一致性。
func (m Manifest) VerifyDigest(expectedDigest string) error {
	if m.FormatVersion != FormatVersion || !strings.EqualFold(m.Digest, expectedDigest) {
		return fmt.Errorf("制品清单版本或摘要不匹配")
	}
	actualDigest, err := m.CalculateDigest()
	if err != nil {
		return fmt.Errorf("计算制品清单摘要失败: %w", err)
	}
	if !strings.EqualFold(m.Digest, actualDigest) {
		return fmt.Errorf("制品清单内容摘要校验失败")
	}
	return nil
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
