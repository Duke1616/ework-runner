package artifact

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	artifactarchive "github.com/Duke1616/etask/internal/artifact/archive"
	executorartifact "github.com/Duke1616/etask/sdk/executor/artifact"
)

// layerRef 是执行侧校验并规范化后的不可变制品引用。
type layerRef struct {
	kind          layerKind
	releaseID     int64
	sourceID      int64
	digest        string
	blobChecksum  string
	size          int64
	format        string
	formatVersion int32
	mountName     string
}

type layerKind uint8

const (
	layerArtifact layerKind = iota + 1
	layerProjectSource
)

type layerSet struct {
	sourceLayer  *layerRef
	defaultLayer layerRef
	hasDefault   bool
	namedLayers  []layerRef
}

type cacheMarker struct {
	Digest        string `json:"digest"`
	BlobChecksum  string `json:"blob_checksum"`
	Size          int64  `json:"size"`
	Format        string `json:"format"`
	FormatVersion int32  `json:"format_version"`
}

func parseLayerSet(source *executorartifact.SourceRef, refs []executorartifact.Ref) (layerSet, error) {
	result := layerSet{namedLayers: make([]layerRef, 0, len(refs))}
	if source != nil {
		ref, err := parseSourceLayerRef(*source)
		if err != nil {
			return layerSet{}, err
		}
		result.sourceLayer = &ref
	}
	namespaces := make(map[string]struct{}, len(refs))
	for _, value := range refs {
		ref, err := parseArtifactLayerRef(value)
		if err != nil {
			return layerSet{}, err
		}
		if ref.mountName == "" {
			if result.hasDefault {
				return layerSet{}, fmt.Errorf("任务包含重复的默认制品层")
			}
			result.defaultLayer = ref
			result.hasDefault = true
			continue
		}
		if err = validateMountName(ref.mountName); err != nil {
			return layerSet{}, err
		}
		if _, exists := namespaces[ref.mountName]; exists {
			return layerSet{}, fmt.Errorf("任务包含重复的制品挂载名称: %s", ref.mountName)
		}
		namespaces[ref.mountName] = struct{}{}
		result.namedLayers = append(result.namedLayers, ref)
	}
	return result, nil
}

func parseArtifactLayerRef(value executorartifact.Ref) (layerRef, error) {
	if value.ReleaseID <= 0 {
		return layerRef{}, fmt.Errorf("制品发布 ID 非法")
	}
	if value.Size <= 0 {
		return layerRef{}, fmt.Errorf("制品大小非法: %d", value.Size)
	}
	digest, err := normalizeDigest(value.Digest)
	if err != nil {
		return layerRef{}, err
	}
	checksum, err := normalizeDigest(value.BlobChecksum)
	if err != nil {
		return layerRef{}, fmt.Errorf("制品压缩包校验和非法: %w", err)
	}
	if !artifactarchive.Supports(value.Format, value.FormatVersion) {
		return layerRef{}, fmt.Errorf("不支持的制品格式: %s/%d", value.Format, value.FormatVersion)
	}
	return layerRef{
		kind: layerArtifact, releaseID: value.ReleaseID, digest: digest, blobChecksum: checksum,
		size: value.Size, format: value.Format, formatVersion: value.FormatVersion,
		mountName: value.MountName,
	}, nil
}

func parseSourceLayerRef(value executorartifact.SourceRef) (layerRef, error) {
	if value.SourceID <= 0 {
		return layerRef{}, fmt.Errorf("项目源码 ID 非法")
	}
	if value.Size <= 0 {
		return layerRef{}, fmt.Errorf("项目源码大小非法: %d", value.Size)
	}
	digest, err := normalizeDigest(value.Digest)
	if err != nil {
		return layerRef{}, err
	}
	checksum, err := normalizeDigest(value.BlobChecksum)
	if err != nil {
		return layerRef{}, fmt.Errorf("项目源码压缩包校验和非法: %w", err)
	}
	if !artifactarchive.Supports(value.Format, value.FormatVersion) {
		return layerRef{}, fmt.Errorf("不支持的项目源码格式: %s/%d", value.Format, value.FormatVersion)
	}
	return layerRef{
		kind: layerProjectSource, sourceID: value.SourceID,
		digest: digest, blobChecksum: checksum, size: value.Size,
		format: value.Format, formatVersion: value.FormatVersion,
	}, nil
}

func normalizeDigest(value string) (string, error) {
	if len(value) != 64 {
		return "", fmt.Errorf("制品摘要长度非法")
	}
	if _, err := hex.DecodeString(value); err != nil {
		return "", fmt.Errorf("制品摘要格式非法")
	}
	return strings.ToLower(value), nil
}

func validateMountName(name string) error {
	if name == "." || name == ".." || filepath.Base(name) != name || name == "etask" {
		return fmt.Errorf("制品挂载名称非法或使用了运行时保留名: %q", name)
	}
	return nil
}

func (r layerRef) cacheKey() string {
	return r.digest + "-" + r.blobChecksum
}

func (r layerRef) artifactRef() executorartifact.Ref {
	return executorartifact.Ref{
		ReleaseID: r.releaseID, Digest: r.digest, BlobChecksum: r.blobChecksum,
		Size: r.size, Format: r.format, FormatVersion: r.formatVersion, MountName: r.mountName,
	}
}

func (r layerRef) sourceRef() executorartifact.SourceRef {
	return executorartifact.SourceRef{
		SourceID: r.sourceID, Digest: r.digest, BlobChecksum: r.blobChecksum,
		Size: r.size, Format: r.format, FormatVersion: r.formatVersion,
	}
}

func (r layerRef) metadata() artifactarchive.Metadata {
	return artifactarchive.Metadata{Digest: r.digest, Format: r.format, FormatVersion: r.formatVersion}
}

func (r layerRef) marker() cacheMarker {
	return cacheMarker{
		Digest: r.digest, BlobChecksum: r.blobChecksum, Size: r.size,
		Format: r.format, FormatVersion: r.formatVersion,
	}
}

func readyArtifact(dir string, ref layerRef) bool {
	data, err := os.ReadFile(filepath.Join(dir, ".ready"))
	if err != nil {
		return false
	}
	var marker cacheMarker
	return json.Unmarshal(data, &marker) == nil && marker == ref.marker()
}
