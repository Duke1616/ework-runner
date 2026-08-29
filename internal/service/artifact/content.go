package artifact

import (
	"context"
	"fmt"

	artifactarchive "github.com/Duke1616/etask/internal/artifact/archive"
	"github.com/Duke1616/etask/internal/domain"
	"github.com/samber/lo"
)

// ActiveContents 查询当前执行会使用的全部激活制品及其文件清单。
func (s *service) ActiveContents(ctx context.Context, sourceProjectID int64) ([]domain.ArtifactContent, error) {
	// 复用执行解析规则，确保页面看到的清单与任务真正注入的制品一致。
	refs, err := s.ResolveExecution(ctx, sourceProjectID)
	if err != nil {
		return nil, err
	}
	contents := make([]domain.ArtifactContent, 0, len(refs))
	for _, ref := range refs {
		reader, openErr := s.Open(ctx, ref.ReleaseID, ref.Digest)
		if openErr != nil {
			return nil, openErr
		}
		manifest, readErr := s.archive.ReadManifest(reader, ref.Digest)
		closeErr := reader.Close()
		if readErr != nil {
			return nil, fmt.Errorf("读取制品 %d 清单失败: %w", ref.ReleaseID, readErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("关闭制品 %d 数据流失败: %w", ref.ReleaseID, closeErr)
		}
		files := lo.Map(manifest.Files, func(file artifactarchive.ManifestFile, _ int) domain.ArtifactManifestFile {
			return domain.ArtifactManifestFile{Path: file.Path, Hash: file.Hash, Size: file.Size}
		})
		contents = append(contents, domain.ArtifactContent{
			Release: domain.ArtifactRelease{
				ID: ref.ReleaseID, Scope: ref.Scope, ProjectID: ref.ProjectID,
				Namespace: ref.Namespace, Digest: ref.Digest,
			},
			Files: files,
		})
	}
	return contents, nil
}

// ReadFile 从指定制品发布中读取一个文件的不可变内容。
func (s *service) ReadFile(ctx context.Context, releaseID int64, digest, filePath string) (string, error) {
	reader, err := s.Open(ctx, releaseID, digest)
	if err != nil {
		return "", err
	}
	defer reader.Close()
	return s.archive.ReadFile(reader, digest, filePath)
}
