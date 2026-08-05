package artifact_test

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/Duke1616/eiam/pkg/ctxutil"
	artifactarchive "github.com/Duke1616/etask/internal/artifact/archive"
	"github.com/Duke1616/etask/internal/domain"
	repositorymocks "github.com/Duke1616/etask/internal/repository/mocks"
	artifactsvc "github.com/Duke1616/etask/internal/service/artifact"
	"github.com/Duke1616/etask/pkg/blobstore"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gorm.io/gorm"
)

func TestServiceOpenTranslatesMissingRelease(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymocks.NewMockArtifactRepository(ctrl)
	repo.EXPECT().FindByID(gomock.Any(), int64(1)).Return(domain.ArtifactRelease{}, gorm.ErrRecordNotFound)
	svc := artifactsvc.NewService(repo, artifactStoreStub{}, artifactarchive.New(""))

	_, err := svc.Open(context.Background(), 1, strings.Repeat("a", 64))
	require.ErrorIs(t, err, blobstore.ErrNotFound)
}

func TestServiceStatusRequiresArtifactProject(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymocks.NewMockArtifactRepository(ctrl)
	svc := artifactsvc.NewService(repo, artifactStoreStub{}, artifactarchive.New(""))

	repo.EXPECT().FindActive(gomock.Any(), domain.ArtifactTarget{Scope: domain.CodebookScopeSystem}).
		Return(domain.ArtifactRelease{}, gorm.ErrRecordNotFound)
	_, err := svc.Status(context.Background(), domain.ArtifactTarget{Scope: domain.CodebookScopeSystem})
	require.NoError(t, err)

	repo.EXPECT().GetProject(gomock.Any(), int64(7)).Return(domain.CodebookProject{
		ID: 7, ArtifactEnabled: false, SourceRevision: 4,
	}, nil)
	_, err = svc.Status(context.Background(), domain.ArtifactTarget{
		Scope: domain.CodebookScopeTenant, ProjectID: 7,
	})
	require.ErrorContains(t, err, "当前项目不是制品库")

	tenantTarget := domain.ArtifactTarget{Scope: domain.CodebookScopeTenant, ProjectID: 7}
	repo.EXPECT().GetProject(gomock.Any(), int64(7)).Return(domain.CodebookProject{
		ID: 7, ArtifactEnabled: true, ArtifactNamespace: "ops_common", SourceRevision: 4,
	}, nil)
	repo.EXPECT().FindActive(gomock.Any(), tenantTarget).
		Return(domain.ArtifactRelease{}, gorm.ErrRecordNotFound)
	projectStatus, err := svc.Status(context.Background(), domain.ArtifactTarget{
		Scope: domain.CodebookScopeTenant, ProjectID: 7,
	})
	require.NoError(t, err)
	require.Equal(t, int64(4), projectStatus.SourceRevision)
}

func TestServiceResolveExecutionLayers(t *testing.T) {
	ctrl := gomock.NewController(t)
	systemTarget := domain.ArtifactTarget{Scope: domain.CodebookScopeSystem}
	repo := repositorymocks.NewMockArtifactRepository(ctrl)
	systemRelease := domain.ArtifactRelease{
		ID: 1, Scope: domain.CodebookScopeSystem, Digest: strings.Repeat("a", 64),
		BlobChecksum: strings.Repeat("b", 64), Size: 1, Format: "tar.zst", FormatVersion: 1,
	}
	libraries := []domain.ArtifactRelease{
		{ID: 2, Scope: domain.CodebookScopeTenant, ProjectID: 7, Namespace: "ops_common", Digest: strings.Repeat("c", 64), BlobChecksum: strings.Repeat("d", 64), Size: 1, Format: "tar.zst", FormatVersion: 1},
		{ID: 3, Scope: domain.CodebookScopeTenant, ProjectID: 9, Namespace: "db_common", Digest: strings.Repeat("e", 64), BlobChecksum: strings.Repeat("f", 64), Size: 1, Format: "tar.zst", FormatVersion: 1},
	}
	repo.EXPECT().FindActive(gomock.Any(), systemTarget).Return(systemRelease, nil)
	repo.EXPECT().ListActiveLibraries(gomock.Any()).Return(libraries, nil)
	svc := artifactsvc.NewService(repo, artifactStoreStub{}, artifactarchive.New(""))

	refs, err := svc.ResolveExecution(context.Background(), 7)
	require.NoError(t, err)
	require.Len(t, refs, 2)
	require.Equal(t, domain.CodebookScopeSystem, refs[0].Scope)
	require.Equal(t, int64(9), refs[1].ProjectID)
}

func TestServiceRejectsUnauthorizedArtifactWrite(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymocks.NewMockArtifactRepository(ctrl)
	svc := artifactsvc.NewService(repo, artifactStoreStub{}, artifactarchive.New(""))
	systemTarget := domain.ArtifactTarget{Scope: domain.CodebookScopeSystem}
	projectTarget := domain.ArtifactTarget{Scope: domain.CodebookScopeTenant, ProjectID: 7}

	err := svc.Activate(ctxutil.WithTenantID(context.Background(), 10), systemTarget, 1)
	require.ErrorContains(t, err, "只有系统租户")
	err = svc.Activate(context.Background(), projectTarget, 1)
	require.ErrorContains(t, err, "缺少租户上下文")
	repo.EXPECT().GetProject(gomock.Any(), int64(7)).Return(domain.CodebookProject{
		ID: 7, ArtifactEnabled: true, ArtifactNamespace: "ops_common",
	}, nil)
	repo.EXPECT().Activate(gomock.Any(), projectTarget, int64(1)).Return(nil)
	err = svc.Activate(ctxutil.WithTenantID(context.Background(), 10), projectTarget, 1)
	require.NoError(t, err)
}

type artifactStoreStub struct{}

func (artifactStoreStub) Put(context.Context, string, io.Reader, int64, string) error {
	return nil
}

func (artifactStoreStub) Open(context.Context, string) (io.ReadCloser, error) {
	return nil, blobstore.ErrNotFound
}
