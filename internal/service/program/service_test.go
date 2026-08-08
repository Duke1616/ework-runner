package program_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Duke1616/eiam/pkg/ctxutil"
	artifactarchive "github.com/Duke1616/etask/internal/artifact/archive"
	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/errs"
	repositorymocks "github.com/Duke1616/etask/internal/repository/mocks"
	program "github.com/Duke1616/etask/internal/service/program"
	blobstoremocks "github.com/Duke1616/etask/pkg/blobstore/mocks"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gorm.io/gorm"
)

type codebooks struct{ values map[int64]domain.Codebook }

func (c codebooks) GetByID(_ context.Context, id int64) (domain.Codebook, error) {
	return c.values[id], nil
}

func TestResolveInlineCodebook(t *testing.T) {
	svc := program.NewService(codebooks{values: map[int64]domain.Codebook{
		11: {ID: 11, ProjectID: 9, Kind: domain.CodebookKindFile, Code: "print('ok')"},
	}}, nil, nil, nil)
	got, err := svc.Resolve(t.Context(), &domain.ProgramSpec{Kind: domain.ProgramInline,
		Inline: &domain.InlineProgramSpec{CodebookID: 11}})
	require.NoError(t, err)
	require.Equal(t, "print('ok')", got.Program.Inline.Code)
	require.Equal(t, int64(9), got.SourceProjectID)
}

func TestResolveInlineRejectsBlobCodebook(t *testing.T) {
	svc := program.NewService(codebooks{values: map[int64]domain.Codebook{
		11: {ID: 11, ProjectID: 9, Kind: domain.CodebookKindFile, StorageType: domain.CodebookContentBlob},
	}}, nil, nil, nil)
	_, err := svc.Resolve(t.Context(), &domain.ProgramSpec{Kind: domain.ProgramInline,
		Inline: &domain.InlineProgramSpec{CodebookID: 11}})
	require.ErrorContains(t, err, "PROJECT")
}

func TestResolveProject(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymocks.NewMockProjectSourceRepository(ctrl)
	store := blobstoremocks.NewMockStore(ctrl)
	repo.EXPECT().GetProject(gomock.Any(), int64(9)).Return(domain.CodebookProject{
		ID: 9, Scope: domain.CodebookScopeTenant, SourceRevision: 4,
	}, nil)
	source := domain.ProjectSource{ID: 21, ProjectID: 9, SourceRevision: 4,
		Digest: strings.Repeat("a", 64), BlobChecksum: strings.Repeat("b", 64),
		Size: 128, Format: "tar.zst", FormatVersion: 1}
	repo.EXPECT().FindByRevision(gomock.Any(), int64(9), int64(4)).Return(source, nil)
	codebookReader := codebooks{values: map[int64]domain.Codebook{
		11: {ID: 11, ProjectID: 9, ParentID: 10, Depth: 1, Name: "deploy.yml", Kind: domain.CodebookKindFile, Scope: domain.CodebookScopeTenant},
		10: {ID: 10, ProjectID: 9, Name: "playbooks", Kind: domain.CodebookKindDirectory, Scope: domain.CodebookScopeTenant},
	}}
	svc := program.NewService(codebookReader, repo, store, artifactarchive.New(""))
	got, err := svc.Resolve(ctxutil.WithTenantID(t.Context(), 10), &domain.ProgramSpec{Kind: domain.ProgramProject,
		Project: &domain.ProjectProgramSpec{EntryCodebookID: 11}})
	require.NoError(t, err)
	require.Equal(t, "playbooks/deploy.yml", got.Program.Project.EntryPoint)
	require.Equal(t, int64(21), got.Program.Project.Source.SourceID)
}

func TestResolveProjectRequiresSource(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymocks.NewMockProjectSourceRepository(ctrl)
	repo.EXPECT().GetProject(gomock.Any(), int64(9)).Return(domain.CodebookProject{ID: 9, Scope: domain.CodebookScopeTenant, SourceRevision: 1}, nil)
	repo.EXPECT().FindByRevision(gomock.Any(), int64(9), int64(1)).Return(domain.ProjectSource{}, gorm.ErrRecordNotFound)
	repo.EXPECT().SourceFiles(gomock.Any(), domain.ArtifactTarget{
		Scope: domain.CodebookScopeTenant, ProjectID: 9,
	}).Return(nil, int64(0), errors.New("read failed"))
	svc := program.NewService(codebooks{values: map[int64]domain.Codebook{
		11: {ID: 11, ProjectID: 9, Name: "main.yml", Kind: domain.CodebookKindFile, Scope: domain.CodebookScopeTenant},
	}}, repo, blobstoremocks.NewMockStore(ctrl), artifactarchive.New(""))
	_, err := svc.Resolve(ctxutil.WithTenantID(t.Context(), 10), &domain.ProgramSpec{Kind: domain.ProgramProject,
		Project: &domain.ProjectProgramSpec{EntryCodebookID: 11}})
	require.ErrorContains(t, err, "read failed")
}

func TestResolveProjectReportsArchivedProjectAsUnavailable(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymocks.NewMockProjectSourceRepository(ctrl)
	repo.EXPECT().GetProject(gomock.Any(), int64(9)).Return(domain.CodebookProject{}, gorm.ErrRecordNotFound)
	svc := program.NewService(codebooks{values: map[int64]domain.Codebook{
		11: {ID: 11, ProjectID: 9, Name: "main.yml", Kind: domain.CodebookKindFile,
			Scope: domain.CodebookScopeTenant},
	}}, repo, blobstoremocks.NewMockStore(ctrl), artifactarchive.New(""))

	_, err := svc.Resolve(ctxutil.WithTenantID(t.Context(), 10), &domain.ProgramSpec{
		Kind: domain.ProgramProject, Project: &domain.ProjectProgramSpec{EntryCodebookID: 11},
	})

	require.ErrorIs(t, err, errs.ErrProgramSourceUnavailable)
	require.ErrorContains(t, err, "不存在或已归档")
}
