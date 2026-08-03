package artifact

import (
	"context"
	"io"
	"os"
	"testing"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/service/artifact/packer"
	"github.com/stretchr/testify/require"
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
	packed, err := packer.New(t.TempDir()).Pack([]domain.ArtifactFile{
		{Path: "scripts/common.sh", Code: "echo immutable\n"},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Remove(packed.Path) })

	release := domain.ArtifactRelease{
		ID: 7, Scope: domain.CodebookScopeSystem, Digest: packed.Digest,
		BlobChecksum: packed.BlobChecksum, Size: packed.Size,
		Format: packer.Format, FormatVersion: packer.FormatVersion, ObjectKey: "release.tar.zst",
	}
	repo := artifactRepositoryStub{
		activeByTarget: map[domain.ArtifactTarget]domain.ArtifactRelease{
			{Scope: domain.CodebookScopeSystem}: release,
		},
		findByID: release,
	}
	service := NewService(repo, artifactFileStore{path: packed.Path}, packer.New(""))

	contents, err := service.ActiveContents(t.Context(), 0)
	require.NoError(t, err)
	require.Len(t, contents, 1)
	require.Equal(t, "scripts/common.sh", contents[0].Files[0].Path)

	code, err := service.ReadFile(t.Context(), release.ID, release.Digest, "scripts/common.sh")
	require.NoError(t, err)
	require.Equal(t, "echo immutable\n", code)
}
