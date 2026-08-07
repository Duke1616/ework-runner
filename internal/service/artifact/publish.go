package artifact

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/Duke1616/etask/internal/domain"
)

func (s *service) Publish(ctx context.Context, target domain.ArtifactTarget,
	message string) (domain.ArtifactRelease, error) {
	// 写目标校验同时确定对象存储隔离所需的租户 ID。
	tenantID, err := s.validateWriteTarget(ctx, target)
	if err != nil {
		return domain.ArtifactRelease{}, err
	}
	return s.publish(ctx, target, tenantID, message)
}

func (s *service) publish(ctx context.Context, target domain.ArtifactTarget,
	tenantID int64, message string) (domain.ArtifactRelease, error) {
	// 从当前版本生成一致性源码快照，再打包成内容寻址的不可变对象。
	files, sourceRevision, err := s.repo.SnapshotFiles(ctx, target)
	if err != nil {
		return domain.ArtifactRelease{}, err
	}
	packed, err := s.archive.Pack(files)
	if err != nil {
		return domain.ArtifactRelease{}, err
	}
	defer os.Remove(packed.Path)

	// 对象键包含租户、目标和内容摘要，相同内容可安全复用且不会跨租户冲突。
	file, err := os.Open(packed.Path)
	if err != nil {
		return domain.ArtifactRelease{}, fmt.Errorf("打开待上传制品失败: %w", err)
	}
	defer file.Close()
	objectKey := fmt.Sprintf("artifacts/%d/%s/%d/%s.%s", tenantID, target.Scope,
		target.ProjectID, packed.Digest, packed.Format)
	if err = s.store.Put(ctx, objectKey, file, packed.Size, packed.BlobChecksum); err != nil {
		return domain.ArtifactRelease{}, fmt.Errorf("保存制品失败: %w", err)
	}
	// 数据对象写入成功后再创建发布记录，数据库事务负责激活版本和并发校验。
	release := domain.ArtifactRelease{
		TenantID: tenantID, Scope: target.Scope, ProjectID: target.ProjectID,
		SourceRevision: sourceRevision, Digest: packed.Digest, BlobChecksum: packed.BlobChecksum,
		ObjectKey: objectKey, Size: packed.Size, Format: packed.Format,
		FormatVersion: packed.FormatVersion, Message: strings.TrimSpace(message),
		AuthorUserID: ctxutil.GetUserID(ctx).Int64(),
	}
	created, err := s.repo.CreateAndActivate(ctx, release)
	if err != nil {
		return domain.ArtifactRelease{}, fmt.Errorf("保存并激活制品发布记录失败: %w", err)
	}
	return created, nil
}
