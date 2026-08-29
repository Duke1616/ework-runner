package artifact

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/pkg/blobstore"
	"github.com/samber/lo"
	"gorm.io/gorm"
)

func (s *service) ResolveExecution(ctx context.Context, sourceProjectID int64) ([]domain.ArtifactRef, error) {
	refs := make([]domain.ArtifactRef, 0, 1)
	// SYSTEM 是固定默认层，存在激活版本时始终优先加入。
	system, err := s.Active(ctx, domain.ArtifactTarget{Scope: domain.CodebookScopeSystem})
	if err != nil {
		return nil, err
	}
	if system != nil {
		refs = append(refs, system.Ref())
	}
	// 租户制品库作为具名依赖层注入，来源项目自身不重复依赖自己。
	libraries, err := s.repo.ListActiveLibraries(ctx)
	if err != nil {
		return nil, err
	}
	tenantRefs := lo.FilterMap(libraries, func(release domain.ArtifactRelease, _ int) (domain.ArtifactRef, bool) {
		if release.ProjectID == sourceProjectID {
			return domain.ArtifactRef{}, false
		}
		return release.Ref(), true
	})
	refs = append(refs, tenantRefs...)

	// 统一校验默认层数量和租户命名空间冲突，避免问题延迟到 Executor。
	if err = domain.ValidateArtifactRefs(refs); err != nil {
		return nil, fmt.Errorf("执行制品配置非法: %w", err)
	}
	return refs, nil
}

func (s *service) Open(ctx context.Context, releaseID int64, digest string) (io.ReadCloser, error) {
	if releaseID <= 0 || !validDigest(digest) {
		return nil, fmt.Errorf("制品引用非法")
	}
	release, err := s.repo.FindByID(ctx, releaseID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, blobstore.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询制品失败: %w", err)
	}
	// 发布 ID 和摘要必须同时匹配，防止调用方用合法 ID 读取其他内容。
	if !strings.EqualFold(release.Digest, digest) {
		return nil, blobstore.ErrNotFound
	}
	reader, err := s.store.Open(ctx, release.ObjectKey)
	if err != nil {
		return nil, fmt.Errorf("打开制品数据失败: %w", err)
	}
	return reader, nil
}

func validDigest(digest string) bool {
	digest = strings.TrimSpace(digest)
	if len(digest) != 64 {
		return false
	}
	_, err := hex.DecodeString(digest)
	return err == nil
}
