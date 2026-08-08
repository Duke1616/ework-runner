package codebook

import (
	"context"
	"errors"
	"testing"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/errs"
	"github.com/Duke1616/etask/internal/repository"
	"github.com/stretchr/testify/require"
)

func TestCodebookNameConflict(t *testing.T) {
	testCases := []struct {
		name     string
		err      error
		fileName string
		want     string
	}{
		{name: "补充冲突文件名", err: errs.ErrCodebookNameConflict, fileName: "deploy.sh", want: "同级目录下已存在同名文件或目录：deploy.sh"},
		{name: "保留其他错误", err: errs.ErrInvalidParameter, fileName: "deploy.sh", want: "参数非法"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got := codebookNameConflict(testCase.err, testCase.fileName)
			if got.Error() != testCase.want {
				t.Fatalf("codebookNameConflict() = %q, 期望 %q", got, testCase.want)
			}
			if errors.Is(testCase.err, errs.ErrCodebookNameConflict) &&
				!errors.Is(got, errs.ErrCodebookNameConflict) {
				t.Fatal("转换后的错误未保留 Codebook 名称冲突语义")
			}
		})
	}
}

type projectRepositoryStub struct {
	repository.ICodebookRepository
	projects        []domain.CodebookProject
	total           int64
	listStatus      domain.CodebookProjectStatus
	totalStatus     domain.CodebookProjectStatus
	archivedID      int64
	restoredID      int64
	archiveAffected int64
	restoreAffected int64
}

func (s *projectRepositoryStub) ListProjects(_ context.Context, status domain.CodebookProjectStatus,
	_, _ int64) ([]domain.CodebookProject, error) {
	s.listStatus = status
	return s.projects, nil
}

func (s *projectRepositoryStub) TotalProjects(_ context.Context,
	status domain.CodebookProjectStatus) (int64, error) {
	s.totalStatus = status
	return s.total, nil
}

func (s *projectRepositoryStub) ArchiveProject(_ context.Context, id int64) (int64, error) {
	s.archivedID = id
	return s.archiveAffected, nil
}

func (s *projectRepositoryStub) RestoreProject(_ context.Context, id int64) (int64, error) {
	s.restoredID = id
	return s.restoreAffected, nil
}

func TestListProjectsDefaultsToNormalStatus(t *testing.T) {
	repo := &projectRepositoryStub{
		projects: []domain.CodebookProject{{ID: 1, Status: domain.CodebookProjectStatusNormal}},
		total:    1,
	}
	svc := NewService(repo)

	projects, total, err := svc.ListProjects(t.Context(), "", 0, 20)

	require.NoError(t, err)
	require.Len(t, projects, 1)
	require.Equal(t, int64(1), total)
	require.Equal(t, domain.CodebookProjectStatusNormal, repo.listStatus)
	require.Equal(t, domain.CodebookProjectStatusNormal, repo.totalStatus)
}

func TestListProjectsAcceptsArchivedStatus(t *testing.T) {
	repo := &projectRepositoryStub{}
	svc := NewService(repo)

	_, _, err := svc.ListProjects(t.Context(), domain.CodebookProjectStatusArchived, 0, 20)

	require.NoError(t, err)
	require.Equal(t, domain.CodebookProjectStatusArchived, repo.listStatus)
	require.Equal(t, domain.CodebookProjectStatusArchived, repo.totalStatus)
}

func TestListProjectsRejectsInvalidStatus(t *testing.T) {
	svc := NewService(&projectRepositoryStub{})

	_, _, err := svc.ListProjects(t.Context(), domain.CodebookProjectStatus("DELETED"), 0, 20)

	require.ErrorIs(t, err, errs.ErrInvalidParameter)
}

func TestArchiveAndRestoreProject(t *testing.T) {
	repo := &projectRepositoryStub{archiveAffected: 1, restoreAffected: 1}
	svc := NewService(repo)

	archived, err := svc.ArchiveProject(t.Context(), 7)
	require.NoError(t, err)
	require.Equal(t, int64(1), archived)
	require.Equal(t, int64(7), repo.archivedID)

	restored, err := svc.RestoreProject(t.Context(), 7)
	require.NoError(t, err)
	require.Equal(t, int64(1), restored)
	require.Equal(t, int64(7), repo.restoredID)
}
