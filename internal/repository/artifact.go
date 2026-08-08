package repository

import (
	"context"
	"fmt"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/repository/dao"
	"github.com/samber/lo"
)

//go:generate go tool mockgen -source=./artifact.go -package=repositorymocks -destination=./mocks/artifact.mock.go -typed

type ArtifactRepository interface {
	// SnapshotFiles 获取指定制品目标且受租户插件约束的代码文件快照，并恢复文件路径。
	SnapshotFiles(ctx context.Context, target domain.ArtifactTarget) ([]domain.ArtifactFile, int64, error)
	// CreateAndActivate 创建不可变发布记录，并将其设置为当前激活版本。
	CreateAndActivate(ctx context.Context, release domain.ArtifactRelease) (domain.ArtifactRelease, error)
	// FindActive 查询目标当前激活的制品发布记录。
	FindActive(ctx context.Context, target domain.ArtifactTarget) (domain.ArtifactRelease, error)
	// FindByID 根据发布 ID 查询制品发布记录。
	FindByID(ctx context.Context, id int64) (domain.ArtifactRelease, error)
	// List 分页查询目标下的制品发布记录及总数。
	List(ctx context.Context, target domain.ArtifactTarget, offset, limit int64) ([]domain.ArtifactRelease, int64, error)
	// Activate 将指定制品发布记录设置为当前激活版本。
	Activate(ctx context.Context, target domain.ArtifactTarget, id int64) error
	// GetProject 查询制品目标对应的代码项目。
	GetProject(ctx context.Context, projectID int64) (domain.CodebookProject, error)
	// ListActiveLibraries 查询当前租户全部已发布制品库的激活版本。
	ListActiveLibraries(ctx context.Context) ([]domain.ArtifactRelease, error)
}

type artifactRepository struct {
	artifactDAO dao.ArtifactDAO
	codebookDAO dao.CodebookDAO
	projectDAO  dao.CodebookProjectDAO
}

func NewArtifactRepository(artifactDAO dao.ArtifactDAO, codebookDAO dao.CodebookDAO,
	projectDAO dao.CodebookProjectDAO) ArtifactRepository {
	return &artifactRepository{artifactDAO: artifactDAO, codebookDAO: codebookDAO, projectDAO: projectDAO}
}

func (r *artifactRepository) SnapshotFiles(ctx context.Context, target domain.ArtifactTarget) ([]domain.ArtifactFile, int64, error) {
	// 先记录租户项目修订号，用于检测快照期间发生的并发编辑。
	sourceRevision, err := r.getSourceRevision(ctx, target)
	if err != nil {
		return nil, 0, err
	}

	// 节点和当前版本分批读取，再由 artifactSnapshot 恢复树形路径。
	nodes, err := r.codebookDAO.ListByTarget(ctx, target.Scope.String(), target.ProjectID)
	if err != nil {
		return nil, 0, fmt.Errorf("查询代码资源节点失败: %w", err)
	}

	snapshot, err := r.loadArtifactSnapshot(ctx, nodes)
	if err != nil {
		return nil, 0, err
	}
	files, err := snapshot.Files()
	if err != nil {
		return nil, 0, err
	}
	// SYSTEM 由命令行整体导入；租户项目需要二次确认修订号以保证一致性。
	if target.Scope == domain.CodebookScopeTenant {
		currentRevision, revisionErr := r.getSourceRevision(ctx, target)
		if revisionErr != nil {
			return nil, 0, revisionErr
		}
		if currentRevision != sourceRevision {
			return nil, 0, fmt.Errorf("代码项目在制品快照期间发生变更，请重新发布")
		}
	}
	return files, sourceRevision, nil
}

func (r *artifactRepository) getSourceRevision(ctx context.Context, target domain.ArtifactTarget) (int64, error) {
	if target.Scope != domain.CodebookScopeTenant {
		return 0, nil
	}
	project, err := r.projectDAO.GetByID(ctx, target.ProjectID)
	if err != nil {
		return 0, fmt.Errorf("查询代码项目失败: %w", err)
	}
	return project.SourceRevision, nil
}

func (r *artifactRepository) loadArtifactSnapshot(ctx context.Context, nodes []dao.Codebook) (artifactSnapshot, error) {
	versionIDs, err := artifactCurrentVersionIDs(nodes)
	if err != nil {
		return artifactSnapshot{}, err
	}
	versions, err := r.codebookDAO.ListVersionsByIDs(ctx, versionIDs)
	if err != nil {
		return artifactSnapshot{}, fmt.Errorf("查询代码资源当前文件版本失败: %w", err)
	}
	return newArtifactSnapshot(nodes, versions), nil
}

func (r *artifactRepository) CreateAndActivate(ctx context.Context, release domain.ArtifactRelease) (domain.ArtifactRelease, error) {
	created, err := r.artifactDAO.CreateAndActivate(ctx, toArtifactEntity(release))
	return toArtifactDomain(created), err
}

func (r *artifactRepository) FindActive(ctx context.Context, target domain.ArtifactTarget) (domain.ArtifactRelease, error) {
	release, err := r.artifactDAO.FindActive(ctx, target.Scope.String(), target.ProjectID)
	return toArtifactDomain(release), err
}

func (r *artifactRepository) FindByID(ctx context.Context, id int64) (domain.ArtifactRelease, error) {
	release, err := r.artifactDAO.FindByID(ctx, id)
	return toArtifactDomain(release), err
}

func (r *artifactRepository) List(ctx context.Context, target domain.ArtifactTarget, offset, limit int64) ([]domain.ArtifactRelease, int64, error) {
	releases, total, err := r.artifactDAO.List(ctx, target.Scope.String(), target.ProjectID, offset, limit)
	return lo.Map(releases, func(release dao.ArtifactRelease, _ int) domain.ArtifactRelease {
		return toArtifactDomain(release)
	}), total, err
}

func (r *artifactRepository) Activate(ctx context.Context, target domain.ArtifactTarget, id int64) error {
	return r.artifactDAO.Activate(ctx, target.Scope.String(), target.ProjectID, id)
}

func (r *artifactRepository) GetProject(ctx context.Context, projectID int64) (domain.CodebookProject, error) {
	project, err := r.projectDAO.GetByID(ctx, projectID)
	if err != nil {
		return domain.CodebookProject{}, err
	}
	return domain.CodebookProject{
		ID: project.ID, TenantID: project.TenantID, Scope: domain.CodebookScope(project.Scope),
		Name: project.Name, Status: domain.CodebookProjectStatus(project.Status),
		ArtifactEnabled:   project.ArtifactEnabled,
		ArtifactNamespace: artifactNamespace(project), SourceRevision: project.SourceRevision,
	}, nil
}

func (r *artifactRepository) ListActiveLibraries(ctx context.Context) ([]domain.ArtifactRelease, error) {
	// 先读取制品库项目，租户插件会自动限制在当前租户空间。
	projects, err := r.projectDAO.ListArtifactProjects(ctx)
	if err != nil {
		return nil, fmt.Errorf("查询制品库项目失败: %w", err)
	}
	// 批量加载激活版本，避免逐项目查询产生 N+1。
	projectIDs := lo.Map(projects, func(project dao.CodebookProject, _ int) int64 {
		return project.ID
	})
	releases, err := r.artifactDAO.ListActiveByProjectIDs(ctx, projectIDs)
	if err != nil {
		return nil, fmt.Errorf("查询制品库当前发布版本失败: %w", err)
	}
	releaseByProject := lo.SliceToMap(releases, func(release dao.ArtifactRelease) (int64, dao.ArtifactRelease) {
		return release.ProjectID, release
	})
	// 按项目稳定顺序组装，并把项目英文名称转换为运行时命名空间。
	result := make([]domain.ArtifactRelease, 0, len(releases))
	for _, project := range projects {
		if release, ok := releaseByProject[project.ID]; ok {
			value := toArtifactDomain(release)
			value.Namespace = artifactNamespace(project)
			result = append(result, value)
		}
	}
	return result, nil
}

func artifactNamespace(project dao.CodebookProject) string {
	if project.ArtifactNamespace == nil {
		return ""
	}
	return *project.ArtifactNamespace
}

func toArtifactEntity(release domain.ArtifactRelease) dao.ArtifactRelease {
	return dao.ArtifactRelease{
		ID:             release.ID,
		Scope:          release.Scope.String(),
		ProjectID:      release.ProjectID,
		SourceRevision: release.SourceRevision,
		Digest:         release.Digest,
		BlobChecksum:   release.BlobChecksum,
		ObjectKey:      release.ObjectKey,
		Size:           release.Size,
		Format:         release.Format,
		FormatVersion:  release.FormatVersion,
		Message:        release.Message,
		AuthorUserID:   release.AuthorUserID,
		Active:         release.Active,
		CTime:          release.CTime,
	}
}

func toArtifactDomain(release dao.ArtifactRelease) domain.ArtifactRelease {
	return domain.ArtifactRelease{
		ID:             release.ID,
		TenantID:       release.TenantID,
		Scope:          domain.CodebookScope(release.Scope),
		ProjectID:      release.ProjectID,
		SourceRevision: release.SourceRevision,
		Digest:         release.Digest,
		BlobChecksum:   release.BlobChecksum,
		ObjectKey:      release.ObjectKey,
		Size:           release.Size,
		Format:         release.Format,
		FormatVersion:  release.FormatVersion,
		Message:        release.Message,
		AuthorUserID:   release.AuthorUserID,
		Active:         release.Active,
		CTime:          release.CTime,
	}
}
