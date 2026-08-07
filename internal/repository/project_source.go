package repository

import (
	"context"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/repository/dao"
)

//go:generate go tool mockgen -source=./project_source.go -package=repositorymocks -destination=./mocks/project_source.mock.go -typed

type ProjectSourceRepository interface {
	GetProject(ctx context.Context, projectID int64) (domain.CodebookProject, error)
	SourceFiles(ctx context.Context, target domain.ArtifactTarget) ([]domain.ArtifactFile, int64, error)
	FindByRevision(ctx context.Context, projectID, sourceRevision int64) (domain.ProjectSource, error)
	FindByID(ctx context.Context, id int64) (domain.ProjectSource, error)
	Create(ctx context.Context, source domain.ProjectSource) (domain.ProjectSource, error)
}

type projectSourceRepository struct {
	source ArtifactRepository
	dao    dao.ProjectSourceDAO
}

func NewProjectSourceRepository(source ArtifactRepository,
	sourceDAO dao.ProjectSourceDAO) ProjectSourceRepository {
	return &projectSourceRepository{source: source, dao: sourceDAO}
}

func (r *projectSourceRepository) GetProject(ctx context.Context,
	projectID int64) (domain.CodebookProject, error) {
	return r.source.GetProject(ctx, projectID)
}

func (r *projectSourceRepository) SourceFiles(ctx context.Context,
	target domain.ArtifactTarget) ([]domain.ArtifactFile, int64, error) {
	return r.source.SnapshotFiles(ctx, target)
}

func (r *projectSourceRepository) FindByRevision(ctx context.Context,
	projectID, sourceRevision int64) (domain.ProjectSource, error) {
	value, err := r.dao.FindByRevision(ctx, projectID, sourceRevision)
	return toProjectSourceDomain(value), err
}

func (r *projectSourceRepository) FindByID(ctx context.Context,
	id int64) (domain.ProjectSource, error) {
	value, err := r.dao.FindByID(ctx, id)
	return toProjectSourceDomain(value), err
}

func (r *projectSourceRepository) Create(ctx context.Context,
	source domain.ProjectSource) (domain.ProjectSource, error) {
	value, err := r.dao.Create(ctx, toProjectSourceEntity(source))
	return toProjectSourceDomain(value), err
}

func toProjectSourceEntity(value domain.ProjectSource) dao.ProjectSource {
	return dao.ProjectSource{
		ID: value.ID, TenantID: value.TenantID, ProjectID: value.ProjectID,
		SourceRevision: value.SourceRevision, Digest: value.Digest,
		BlobChecksum: value.BlobChecksum, ObjectKey: value.ObjectKey, Size: value.Size,
		Format: value.Format, FormatVersion: value.FormatVersion, CTime: value.CTime,
	}
}

func toProjectSourceDomain(value dao.ProjectSource) domain.ProjectSource {
	return domain.ProjectSource{
		ID: value.ID, TenantID: value.TenantID, ProjectID: value.ProjectID,
		SourceRevision: value.SourceRevision, Digest: value.Digest,
		BlobChecksum: value.BlobChecksum, ObjectKey: value.ObjectKey, Size: value.Size,
		Format: value.Format, FormatVersion: value.FormatVersion, CTime: value.CTime,
	}
}
