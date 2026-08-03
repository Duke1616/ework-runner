package artifact

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/service/artifact/packer"
	"github.com/Duke1616/etask/pkg/blobstore"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestServiceOpenTranslatesMissingRelease(t *testing.T) {
	repo := artifactRepositoryStub{findByIDErr: gorm.ErrRecordNotFound}
	svc := NewService(repo, artifactStoreStub{}, packer.New(""))

	_, err := svc.Open(context.Background(), 1, strings.Repeat("a", 64))
	require.ErrorIs(t, err, blobstore.ErrNotFound)
}

func TestServiceStatusRequiresArtifactProject(t *testing.T) {
	repo := artifactRepositoryStub{
		project: domain.CodebookProject{ID: 7, ArtifactEnabled: false, SourceRevision: 4},
	}
	svc := NewService(repo, artifactStoreStub{}, packer.New(""))

	_, err := svc.Status(context.Background(), domain.ArtifactTarget{Scope: domain.CodebookScopeSystem})
	require.NoError(t, err)

	_, err = svc.Status(context.Background(), domain.ArtifactTarget{
		Scope: domain.CodebookScopeTenant, ProjectID: 7,
	})
	require.ErrorContains(t, err, "当前项目不是制品库")

	repo.project.ArtifactEnabled = true
	repo.project.ArtifactNamespace = "ops_common"
	svc = NewService(repo, artifactStoreStub{}, packer.New(""))
	projectStatus, err := svc.Status(context.Background(), domain.ArtifactTarget{
		Scope: domain.CodebookScopeTenant, ProjectID: 7,
	})
	require.NoError(t, err)
	require.Equal(t, int64(4), projectStatus.SourceRevision)
}

func TestServiceResolveExecutionLayers(t *testing.T) {
	systemTarget := domain.ArtifactTarget{Scope: domain.CodebookScopeSystem}
	repo := artifactRepositoryStub{
		activeByTarget: map[domain.ArtifactTarget]domain.ArtifactRelease{
			systemTarget: {ID: 1, Scope: domain.CodebookScopeSystem, Digest: strings.Repeat("a", 64), BlobChecksum: strings.Repeat("b", 64), Size: 1, Format: "tar.zst", FormatVersion: 1},
		},
		activeLibraries: []domain.ArtifactRelease{
			{ID: 2, Scope: domain.CodebookScopeTenant, ProjectID: 7, Namespace: "ops_common", Digest: strings.Repeat("c", 64), BlobChecksum: strings.Repeat("d", 64), Size: 1, Format: "tar.zst", FormatVersion: 1},
			{ID: 3, Scope: domain.CodebookScopeTenant, ProjectID: 9, Namespace: "db_common", Digest: strings.Repeat("e", 64), BlobChecksum: strings.Repeat("f", 64), Size: 1, Format: "tar.zst", FormatVersion: 1},
		},
	}
	svc := NewService(repo, artifactStoreStub{}, packer.New(""))

	refs, err := svc.ResolveExecution(context.Background(), 7)
	require.NoError(t, err)
	require.Len(t, refs, 2)
	require.Equal(t, domain.CodebookScopeSystem, refs[0].Scope)
	require.Equal(t, int64(9), refs[1].ProjectID)
}

func TestServiceRejectsUnauthorizedArtifactWrite(t *testing.T) {
	svc := NewService(artifactRepositoryStub{
		project: domain.CodebookProject{ID: 7, ArtifactEnabled: true, ArtifactNamespace: "ops_common"},
	}, artifactStoreStub{}, packer.New(""))
	systemTarget := domain.ArtifactTarget{Scope: domain.CodebookScopeSystem}
	projectTarget := domain.ArtifactTarget{Scope: domain.CodebookScopeTenant, ProjectID: 7}

	err := svc.Activate(ctxutil.WithTenantID(context.Background(), 10), systemTarget, 1)
	require.ErrorContains(t, err, "只有系统租户")
	err = svc.Activate(context.Background(), projectTarget, 1)
	require.ErrorContains(t, err, "缺少租户上下文")
	err = svc.Activate(ctxutil.WithTenantID(context.Background(), 10), projectTarget, 1)
	require.NoError(t, err)
}

type artifactRepositoryStub struct {
	findByIDErr     error
	findByID        domain.ArtifactRelease
	activeByTarget  map[domain.ArtifactTarget]domain.ArtifactRelease
	activeLibraries []domain.ArtifactRelease
	project         domain.CodebookProject
}

func (artifactRepositoryStub) SnapshotFiles(context.Context, domain.ArtifactTarget) ([]domain.ArtifactFile, int64, error) {
	return nil, 0, nil
}

func (artifactRepositoryStub) CreateAndActivate(_ context.Context,
	release domain.ArtifactRelease) (domain.ArtifactRelease, error) {
	return release, nil
}

func (s artifactRepositoryStub) FindActive(_ context.Context, target domain.ArtifactTarget) (domain.ArtifactRelease, error) {
	release, ok := s.activeByTarget[target]
	if !ok {
		return domain.ArtifactRelease{}, gorm.ErrRecordNotFound
	}
	return release, nil
}

func (s artifactRepositoryStub) FindByID(context.Context, int64) (domain.ArtifactRelease, error) {
	return s.findByID, s.findByIDErr
}

func (artifactRepositoryStub) List(context.Context, domain.ArtifactTarget, int64, int64) ([]domain.ArtifactRelease, int64, error) {
	return nil, 0, nil
}

func (artifactRepositoryStub) Activate(context.Context, domain.ArtifactTarget, int64) error {
	return nil
}

func (s artifactRepositoryStub) GetProject(context.Context, int64) (domain.CodebookProject, error) {
	if s.project.ID == 0 {
		return domain.CodebookProject{}, gorm.ErrRecordNotFound
	}
	return s.project, nil
}

func (s artifactRepositoryStub) ListActiveLibraries(context.Context) ([]domain.ArtifactRelease, error) {
	return s.activeLibraries, nil
}

type artifactStoreStub struct{}

func (artifactStoreStub) Put(context.Context, string, io.Reader, int64, string) error {
	return nil
}

func (artifactStoreStub) Open(context.Context, string) (io.ReadCloser, error) {
	return nil, blobstore.ErrNotFound
}
