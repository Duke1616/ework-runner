package domain

import (
	"encoding/hex"
	"fmt"
	"strings"

	artifactv1 "github.com/Duke1616/etask/api/proto/gen/etask/artifact/v1"
	"github.com/Duke1616/etask/internal/errs"
	executorartifact "github.com/Duke1616/etask/sdk/executor/artifact"
)

// ArtifactRef 是一次任务执行所固定的不可变制品引用。
// Digest 标识制品的语义内容，BlobChecksum 用于校验实际传输的压缩包。
type ArtifactRef struct {
	ReleaseID     int64         `json:"releaseId"`
	Digest        string        `json:"digest"`
	BlobChecksum  string        `json:"blobChecksum"`
	Size          int64         `json:"size"`
	Format        string        `json:"format"`
	FormatVersion int32         `json:"formatVersion"`
	Scope         CodebookScope `json:"scope"`
	ProjectID     int64         `json:"projectId"`
	Namespace     string        `json:"namespace"`
}

// Validate 校验不可变制品引用的领域规则和基础元数据。
func (r ArtifactRef) Validate() error {
	if r.ReleaseID <= 0 {
		return fmt.Errorf("%w: 制品发布 ID 非法", errs.ErrInvalidParameter)
	}
	if !validArtifactDigest(r.Digest) || !validArtifactDigest(r.BlobChecksum) {
		return fmt.Errorf("%w: 制品摘要或压缩包校验和非法", errs.ErrInvalidParameter)
	}
	if r.Size <= 0 || strings.TrimSpace(r.Format) == "" || r.FormatVersion <= 0 {
		return fmt.Errorf("%w: 制品格式或大小非法", errs.ErrInvalidParameter)
	}
	if err := (ArtifactTarget{Scope: r.Scope, ProjectID: r.ProjectID}).Validate(); err != nil {
		return err
	}
	switch r.Scope {
	case CodebookScopeSystem:
		if r.Namespace != "" {
			return fmt.Errorf("%w: SYSTEM 制品不能指定导入命名空间", errs.ErrInvalidParameter)
		}
	case CodebookScopeTenant:
		if !artifactNamespacePattern.MatchString(r.Namespace) || r.Namespace == "etask" {
			return fmt.Errorf("%w: 租户制品导入命名空间非法: %s", errs.ErrInvalidParameter, r.Namespace)
		}
	}
	return nil
}

// ToProto 将领域制品引用转换为执行协议引用。
func (r ArtifactRef) ToProto() (*artifactv1.ArtifactRef, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	return &artifactv1.ArtifactRef{
		ReleaseId: r.ReleaseID, Digest: r.Digest, BlobChecksum: r.BlobChecksum,
		Size: r.Size, Format: r.Format, FormatVersion: r.FormatVersion,
		MountName: r.Namespace,
	}, nil
}

// ValidateArtifactRefs 校验一次执行使用的制品层组合。
func ValidateArtifactRefs(refs []ArtifactRef) error {
	systemSeen := false
	projects := make(map[int64]struct{}, len(refs))
	namespaces := make(map[string]int64, len(refs))
	for _, ref := range refs {
		if err := ref.Validate(); err != nil {
			return err
		}
		if ref.Scope == CodebookScopeSystem {
			if systemSeen {
				return fmt.Errorf("%w: 执行制品包含重复的 SYSTEM 层", errs.ErrInvalidParameter)
			}
			systemSeen = true
			continue
		}
		if _, exists := projects[ref.ProjectID]; exists {
			return fmt.Errorf("%w: 执行制品包含重复的租户项目: %d", errs.ErrInvalidParameter, ref.ProjectID)
		}
		projects[ref.ProjectID] = struct{}{}
		if projectID, exists := namespaces[ref.Namespace]; exists {
			return fmt.Errorf("%w: 租户项目 %d 与 %d 使用了重复命名空间: %s",
				errs.ErrInvalidParameter, ref.ProjectID, projectID, ref.Namespace)
		}
		namespaces[ref.Namespace] = ref.ProjectID
	}
	return nil
}

// ArtifactRefsToProto 将领域制品层转换为协议引用。
func ArtifactRefsToProto(refs []ArtifactRef) ([]*artifactv1.ArtifactRef, error) {
	if err := ValidateArtifactRefs(refs); err != nil {
		return nil, err
	}
	result := make([]*artifactv1.ArtifactRef, 0, len(refs))
	for _, ref := range refs {
		value, err := ref.ToProto()
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, nil
}

// ArtifactRefsToExecutor 将领域制品层转换为传输无关的 Executor SDK 引用。
func ArtifactRefsToExecutor(refs []ArtifactRef) ([]executorartifact.Ref, error) {
	if err := ValidateArtifactRefs(refs); err != nil {
		return nil, err
	}
	result := make([]executorartifact.Ref, 0, len(refs))
	for _, ref := range refs {
		result = append(result, executorartifact.Ref{
			ReleaseID: ref.ReleaseID, Digest: ref.Digest, BlobChecksum: ref.BlobChecksum,
			Size: ref.Size, Format: ref.Format, FormatVersion: ref.FormatVersion,
			MountName: ref.Namespace,
		})
	}
	return result, nil
}

func validArtifactDigest(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

// ArtifactTarget 描述制品所属的公共层或租户项目层。
type ArtifactTarget struct {
	Scope     CodebookScope
	ProjectID int64
}

// Validate 校验制品目标的作用域和项目组合。
func (t ArtifactTarget) Validate() error {
	if !t.Scope.Valid() {
		return fmt.Errorf("%w: 不支持的制品作用域: %s", errs.ErrInvalidParameter, t.Scope)
	}
	if t.Scope == CodebookScopeSystem && t.ProjectID != 0 {
		return fmt.Errorf("%w: SYSTEM 制品的项目 ID 必须为 0", errs.ErrInvalidParameter)
	}
	if t.Scope == CodebookScopeTenant && t.ProjectID <= 0 {
		return fmt.Errorf("%w: 租户制品必须指定项目", errs.ErrInvalidParameter)
	}
	return nil
}

// ValidateWriteAccess 校验租户是否可以写入当前制品目标。
func (t ArtifactTarget) ValidateWriteAccess(tenantID, systemTenantID int64) error {
	if err := t.Validate(); err != nil {
		return err
	}
	return t.Scope.ValidateWriteAccess(tenantID, systemTenantID)
}

// ArtifactRelease 是代码资源发布后的不可变制品元数据。
type ArtifactRelease struct {
	ID             int64
	TenantID       int64
	Scope          CodebookScope
	ProjectID      int64
	Namespace      string
	SourceRevision int64
	Digest         string
	BlobChecksum   string
	ObjectKey      string
	Size           int64
	Format         string
	FormatVersion  int32
	Message        string
	AuthorUserID   int64
	Active         bool
	CTime          int64
}

func (r ArtifactRelease) Ref() ArtifactRef {
	return ArtifactRef{
		ReleaseID:     r.ID,
		Digest:        r.Digest,
		BlobChecksum:  r.BlobChecksum,
		Size:          r.Size,
		Format:        r.Format,
		FormatVersion: r.FormatVersion,
		Scope:         r.Scope, ProjectID: r.ProjectID, Namespace: r.Namespace,
	}
}

// ArtifactStatus 描述目标当前的制品配置和发布状态。
type ArtifactStatus struct {
	Target         ArtifactTarget
	SourceRevision int64
	Active         *ArtifactRelease
	PendingChanges bool
}

// ArtifactFile 是构建制品时使用的文件快照。
type ArtifactFile struct {
	Path string
	Hash string
	Code string
}
