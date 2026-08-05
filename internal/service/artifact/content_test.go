package artifact_test

import (
	"context"
	"io"
	"os"
	"testing"

	artifactarchive "github.com/Duke1616/etask/internal/artifact/archive"
	"github.com/Duke1616/etask/internal/domain"
	repositorymocks "github.com/Duke1616/etask/internal/repository/mocks"
	artifactsvc "github.com/Duke1616/etask/internal/service/artifact"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

type artifactFileStore struct {
	path string
}

func (artifactFileStore) Put(context.Context, string, io.Reader, int64, string) error {
	return nil
}

func (s artifactFileStore) Open(context.Context, string) (io.ReadCloser, error) {
	return os.Open(s.path)
}

func TestServiceReadsImmutableArtifactContents(t *testing.T) {
	ctrl := gomock.NewController(t)
	packed, err := artifactarchive.New(t.TempDir()).Pack([]domain.ArtifactFile{
		{Path: "scripts/common.sh", Code: "echo immutable\n"},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Remove(packed.Path) })

	release := domain.ArtifactRelease{
		ID: 7, Scope: domain.CodebookScopeSystem, Digest: packed.Digest,
		BlobChecksum: packed.BlobChecksum, Size: packed.Size,
		Format: packed.Format, FormatVersion: packed.FormatVersion, ObjectKey: "release.tar.zst",
	}
	repo := repositorymocks.NewMockArtifactRepository(ctrl)
	repo.EXPECT().FindActive(gomock.Any(), domain.ArtifactTarget{Scope: domain.CodebookScopeSystem}).
		Return(release, nil)
	repo.EXPECT().ListActiveLibraries(gomock.Any()).Return(nil, nil)
	repo.EXPECT().FindByID(gomock.Any(), release.ID).Return(release, nil).Times(2)
	service := artifactsvc.NewService(repo, artifactFileStore{path: packed.Path}, artifactarchive.New(""))

	contents, err := service.ActiveContents(t.Context(), 0)
	require.NoError(t, err)
	require.Len(t, contents, 1)
	require.Equal(t, "scripts/common.sh", contents[0].Files[0].Path)

	code, err := service.ReadFile(t.Context(), release.ID, release.Digest, "scripts/common.sh")
	require.NoError(t, err)
	require.Equal(t, "echo immutable\n", code)
}
