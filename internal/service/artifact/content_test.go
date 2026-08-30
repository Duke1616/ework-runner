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
	blobstoremocks "github.com/Duke1616/etask/pkg/blobstore/mocks"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestServiceReadsImmutableArtifactContents(t *testing.T) {
	testCases := []struct {
		name     string
		filePath string
		wantCode string
	}{
		{
			name:     "成功_读取激活制品清单与文件源码",
			filePath: "scripts/common.sh",
			wantCode: "echo immutable\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			packed, err := artifactarchive.New(t.TempDir()).Pack([]domain.ArtifactFile{
				{Path: tc.filePath, Code: tc.wantCode},
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

			store := blobstoremocks.NewMockStore(ctrl)
			store.EXPECT().Open(gomock.Any(), release.ObjectKey).DoAndReturn(func(context.Context, string) (io.ReadCloser, error) {
				return os.Open(packed.Path)
			}).Times(2)

			service := artifactsvc.NewService(repo, store, artifactarchive.New(""))

			contents, err := service.ActiveContents(t.Context(), 0)
			require.NoError(t, err)
			require.Len(t, contents, 1)
			require.Equal(t, tc.filePath, contents[0].Files[0].Path)

			code, err := service.ReadFile(t.Context(), release.ID, release.Digest, tc.filePath)
			require.NoError(t, err)
			require.Equal(t, tc.wantCode, code)
		})
	}
}
