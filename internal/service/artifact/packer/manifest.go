package packer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/Duke1616/etask/internal/domain"
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
		actualHash := sha256.Sum256([]byte(files[i].Code))
		actualHashString := hex.EncodeToString(actualHash[:])
		if files[i].Hash != "" && !strings.EqualFold(files[i].Hash, actualHashString) {
			return preparedArtifact{}, fmt.Errorf("制品源文件 %s 的源码校验和与版本记录不一致", clean)
		}
		manifest.Files = append(manifest.Files, ManifestFile{
			Path: clean, Hash: actualHashString, Size: int64(len(files[i].Code)),
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
