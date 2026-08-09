package codebook_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/errs"
	codebookSvc "github.com/Duke1616/etask/internal/service/codebook"
	codebookmocks "github.com/Duke1616/etask/internal/service/codebook/mocks"
	"github.com/Duke1616/etask/pkg/blobstore"
	blobstoremocks "github.com/Duke1616/etask/pkg/blobstore/mocks"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestProjectFileServiceImportsSmallTextInline(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := codebookmocks.NewMockProjectFileRepository(ctrl)
	store := blobstoremocks.NewMockStore(ctrl)
	ctx := ctxutil.WithTenantID(context.Background(), 7)
	repo.EXPECT().GetProjectByID(gomock.Any(), int64(3)).Return(domain.CodebookProject{
		ID: 3, Scope: domain.CodebookScopeTenant,
	}, nil)
	repo.EXPECT().Import(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, request domain.CodebookImport) (domain.CodebookImportResult, error) {
			require.Len(t, request.Files, 1)
			require.Equal(t, "roles/web/tasks/main.yml", request.Files[0].Path)
			require.Equal(t, domain.CodebookContentInline, request.Files[0].StorageType)
			require.Equal(t, "- debug: msg=ok\n", request.Files[0].Code)
			require.True(t, request.Files[0].Overwrite)
			return domain.CodebookImportResult{FileCount: 1, DirectoryCount: 3}, nil
		})

	service := codebookSvc.NewProjectFileService(repo, store)
	content := "- debug: msg=ok\n"
	result, err := service.Import(ctx, codebookSvc.ImportRequest{
		ProjectID:      3,
		OverwritePaths: []string{"roles/web/tasks/main.yml"},
		Files: []codebookSvc.ImportFile{{
			Path: "roles/web/tasks/main.yml", Size: int64(len(content)),
			Open: func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(content)), nil },
		}},
	})
	require.NoError(t, err)
	require.Equal(t, 1, result.FileCount)
}

func TestProjectFileServiceRejectsOverwritePathOutsideUpload(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := codebookmocks.NewMockProjectFileRepository(ctrl)
	store := blobstoremocks.NewMockStore(ctrl)
	ctx := ctxutil.WithTenantID(context.Background(), 7)
	repo.EXPECT().GetProjectByID(gomock.Any(), int64(3)).Return(domain.CodebookProject{
		ID: 3, Scope: domain.CodebookScopeTenant,
	}, nil)

	service := codebookSvc.NewProjectFileService(repo, store)
	_, err := service.Import(ctx, codebookSvc.ImportRequest{
		ProjectID: 3, OverwritePaths: []string{"other.yml"},
		Files: []codebookSvc.ImportFile{{
			Path: "site.yml", Size: 1,
			Open: func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader("x")), nil },
		}},
	})

	require.ErrorIs(t, err, errs.ErrInvalidParameter)
}

func TestProjectFileServiceStoresBinaryInBlob(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := codebookmocks.NewMockProjectFileRepository(ctrl)
	store := blobstoremocks.NewMockStore(ctrl)
	ctx := ctxutil.WithTenantID(context.Background(), 7)
	repo.EXPECT().GetProjectByID(gomock.Any(), int64(3)).Return(domain.CodebookProject{
		ID: 3, Scope: domain.CodebookScopeTenant,
	}, nil)
	store.EXPECT().Put(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, key string, reader io.Reader, options blobstore.PutOptions) error {
			data, err := io.ReadAll(reader)
			require.NoError(t, err)
			require.Equal(t, []byte{0, 1, 2, 3}, data)
			require.Equal(t, int64(4), options.Size)
			require.Contains(t, key, "codebook-content/7/")
			return nil
		})
	repo.EXPECT().Import(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, request domain.CodebookImport) (domain.CodebookImportResult, error) {
			require.Equal(t, domain.CodebookContentBlob, request.Files[0].StorageType)
			require.NotEmpty(t, request.Files[0].ObjectKey)
			require.Empty(t, request.Files[0].Code)
			return domain.CodebookImportResult{FileCount: 1}, nil
		})

	service := codebookSvc.NewProjectFileService(repo, store)
	_, err := service.Import(ctx, codebookSvc.ImportRequest{
		ProjectID: 3,
		Files: []codebookSvc.ImportFile{{
			Path: "files/payload.bin", Size: 4,
			Open: func() (io.ReadCloser, error) {
				return io.NopCloser(strings.NewReader("\x00\x01\x02\x03")), nil
			},
		}},
	})
	require.NoError(t, err)
}

func TestProjectFileServiceDeletesSubtreeBlobs(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := codebookmocks.NewMockProjectFileRepository(ctrl)
	store := blobstoremocks.NewMockStore(ctrl)
	ctx := ctxutil.WithTenantID(context.Background(), 7)
	repo.EXPECT().GetNodeByID(gomock.Any(), int64(10)).Return(domain.Codebook{
		ID: 10, ProjectID: 3, Scope: domain.CodebookScopeTenant, Kind: domain.CodebookKindDirectory,
	}, nil)
	repo.EXPECT().GetProjectByID(gomock.Any(), int64(3)).Return(domain.CodebookProject{
		ID: 3, Status: domain.CodebookProjectStatusNormal,
	}, nil)
	repo.EXPECT().Delete(gomock.Any(), int64(10)).Return(domain.CodebookDeleteResult{
		NodeCount: 3,
		ObjectKeys: []string{
			"codebook-content/7/first",
			"codebook-content/7/second",
		},
	}, nil)
	store.EXPECT().Delete(gomock.Any(), "codebook-content/7/first").Return(nil)
	store.EXPECT().Delete(gomock.Any(), "codebook-content/7/second").Return(nil)

	service := codebookSvc.NewProjectFileService(repo, store)
	count, err := service.Delete(ctx, 10)

	require.NoError(t, err)
	require.Equal(t, int64(3), count)
}

func TestProjectFileServiceReportsBlobDeleteFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := codebookmocks.NewMockProjectFileRepository(ctrl)
	store := blobstoremocks.NewMockStore(ctrl)
	ctx := ctxutil.WithTenantID(context.Background(), 7)
	repo.EXPECT().GetNodeByID(gomock.Any(), int64(10)).Return(domain.Codebook{
		ID: 10, ProjectID: 3, Scope: domain.CodebookScopeTenant, Kind: domain.CodebookKindFile,
	}, nil)
	repo.EXPECT().GetProjectByID(gomock.Any(), int64(3)).Return(domain.CodebookProject{
		ID: 3, Status: domain.CodebookProjectStatusNormal,
	}, nil)
	repo.EXPECT().Delete(gomock.Any(), int64(10)).Return(domain.CodebookDeleteResult{
		NodeCount: 1, ObjectKeys: []string{"codebook-content/7/first"},
	}, nil)
	store.EXPECT().Delete(gomock.Any(), "codebook-content/7/first").Return(errors.New("disk error"))

	service := codebookSvc.NewProjectFileService(repo, store)
	count, err := service.Delete(ctx, 10)

	require.Equal(t, int64(1), count)
	require.ErrorContains(t, err, "删除文件内容 codebook-content/7/first 失败")
}
