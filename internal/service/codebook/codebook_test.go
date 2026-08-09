package codebook

import (
	"context"
	"errors"
	"testing"

	"github.com/Duke1616/eiam/pkg/ctxutil"
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
	projects                  []domain.CodebookProject
	total                     int64
	listQuery                 domain.CodebookProjectListQuery
	countQuery                domain.CodebookProjectListQuery
	archivedID                int64
	restoredID                int64
	archiveAffected           int64
	restoreAffected           int64
	referenceProjects         []domain.CodebookProject
	referenceTotal            int64
	referenceKeyword          string
	referenceExcludeProjectID int64
	referenceOffset           int64
	referenceLimit            int64
	node                      domain.Codebook
	project                   domain.CodebookProject
	changeSet                 domain.CodebookProjectChangeSet
	changeResults             []domain.CodebookProjectChangeResult
}

func (s *projectRepositoryStub) ApplyProjectChangeSet(_ context.Context,
	request domain.CodebookProjectChangeSet) ([]domain.CodebookProjectChangeResult, error) {
	s.changeSet = request
	return s.changeResults, nil
}

func TestApplyProjectChangeSetValidatesAndDelegatesAtomically(t *testing.T) {
	repo := &projectRepositoryStub{
		project: domain.CodebookProject{
			ID: 3, Scope: domain.CodebookScopeTenant, Status: domain.CodebookProjectStatusNormal,
		},
		changeResults: []domain.CodebookProjectChangeResult{{
			Path: "playbooks/site.yml", NodeID: 10, VersionID: 21,
		}},
	}
	service := NewService(repo)
	ctx := ctxutil.WithTenantID(t.Context(), 2)
	request := domain.CodebookProjectChangeSet{
		ProjectID: 3, BaseRevision: 7,
		Changes: []domain.CodebookProjectChange{{
			Operation: domain.CodebookChangeOperationUpdate, Path: "playbooks/site.yml",
			NodeID: 10, ExpectedCurrentVersionID: 20, ExpectedHash: "hash",
			Code: "---\n- hosts: all\n",
		}},
	}

	result, err := service.ApplyProjectChangeSet(ctx, request)

	require.NoError(t, err)
	require.Equal(t, repo.changeResults, result)
	require.Equal(t, request, repo.changeSet)
}

func (s *projectRepositoryStub) ListProjects(_ context.Context,
	query domain.CodebookProjectListQuery) ([]domain.CodebookProject, error) {
	s.listQuery = query
	return s.projects, nil
}

func (s *projectRepositoryStub) CountProjects(_ context.Context,
	query domain.CodebookProjectListQuery) (int64, error) {
	s.countQuery = query
	return s.total, nil
}

func (s *projectRepositoryStub) ListReferenceProjects(_ context.Context, keyword string,
	excludeProjectID, offset, limit int64) ([]domain.CodebookProject, error) {
	s.referenceKeyword = keyword
	s.referenceExcludeProjectID = excludeProjectID
	s.referenceOffset = offset
	s.referenceLimit = limit
	return s.referenceProjects, nil
}

func (s *projectRepositoryStub) CountReferenceProjects(_ context.Context, _ string, _ int64) (int64, error) {
	return s.referenceTotal, nil
}

func (s *projectRepositoryStub) ArchiveProject(_ context.Context, id int64) (int64, error) {
	s.archivedID = id
	return s.archiveAffected, nil
}

func (s *projectRepositoryStub) RestoreProject(_ context.Context, id int64) (int64, error) {
	s.restoredID = id
	return s.restoreAffected, nil
}

func (s *projectRepositoryStub) GetNodeByID(context.Context, int64) (domain.Codebook, error) {
	return s.node, nil
}

func (s *projectRepositoryStub) GetProjectByID(context.Context, int64) (domain.CodebookProject, error) {
	return s.project, nil
}

func TestListProjectsDefaultsToNormalStatus(t *testing.T) {
	repo := &projectRepositoryStub{
		projects: []domain.CodebookProject{{ID: 1, Status: domain.CodebookProjectStatusNormal}},
		total:    1,
	}
	svc := NewService(repo)

	projects, total, err := svc.ListProjects(t.Context(), domain.CodebookProjectListQuery{Limit: 20})

	require.NoError(t, err)
	require.Len(t, projects, 1)
	require.Equal(t, int64(1), total)
	require.Equal(t, domain.CodebookScopeTenant, repo.listQuery.Scope)
	require.Equal(t, domain.CodebookProjectStatusNormal, repo.listQuery.Status)
	require.Equal(t, repo.listQuery, repo.countQuery)
}

func TestListProjectsAcceptsArchivedStatus(t *testing.T) {
	repo := &projectRepositoryStub{}
	svc := NewService(repo)

	_, _, err := svc.ListProjects(t.Context(), domain.CodebookProjectListQuery{
		Status: domain.CodebookProjectStatusArchived, Limit: 20,
	})

	require.NoError(t, err)
	require.Equal(t, domain.CodebookProjectStatusArchived, repo.listQuery.Status)
	require.Equal(t, repo.listQuery, repo.countQuery)
}

func TestListProjectsRejectsInvalidStatus(t *testing.T) {
	svc := NewService(&projectRepositoryStub{})

	_, _, err := svc.ListProjects(t.Context(), domain.CodebookProjectListQuery{
		Status: domain.CodebookProjectStatus("DELETED"), Limit: 20,
	})

	require.ErrorIs(t, err, errs.ErrInvalidParameter)
}

func TestListProjectsAcceptsSystemScope(t *testing.T) {
	repo := &projectRepositoryStub{
		projects: []domain.CodebookProject{{ID: 12, Scope: domain.CodebookScopeSystem, Name: "ansible"}},
		total:    1,
	}
	svc := NewService(repo)

	projects, total, err := svc.ListProjects(t.Context(), domain.CodebookProjectListQuery{
		Scope: domain.CodebookScopeSystem, Offset: 1, Limit: 10,
	})

	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Equal(t, repo.projects, projects)
	require.Equal(t, domain.CodebookProjectStatusNormal, repo.listQuery.Status)
	require.Equal(t, domain.CodebookProjectListQuery{
		Scope: domain.CodebookScopeSystem, Status: domain.CodebookProjectStatusNormal,
		Offset: 1, Limit: 10,
	}, repo.countQuery)
}

func TestListReferenceProjectsTrimsKeywordAndForwardsPage(t *testing.T) {
	repo := &projectRepositoryStub{
		referenceProjects: []domain.CodebookProject{{ID: 9, Status: domain.CodebookProjectStatusArchived}},
		referenceTotal:    12,
	}
	svc := NewService(repo)

	projects, total, err := svc.ListReferenceProjects(t.Context(), "  deploy  ", 7, 20, 10)

	require.NoError(t, err)
	require.Equal(t, repo.referenceProjects, projects)
	require.Equal(t, int64(12), total)
	require.Equal(t, "deploy", repo.referenceKeyword)
	require.Equal(t, int64(7), repo.referenceExcludeProjectID)
	require.Equal(t, int64(20), repo.referenceOffset)
	require.Equal(t, int64(10), repo.referenceLimit)
}

func TestSystemChildrenRejectsParentFromAnotherSystemProject(t *testing.T) {
	svc := NewService(&projectRepositoryStub{node: domain.Codebook{
		ID: 21, Scope: domain.CodebookScopeSystem, PathIDs: "/20/",
	}})

	_, err := svc.SystemChildren(t.Context(), 10, 21)

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

func TestUpdateArchivedProjectIsRejected(t *testing.T) {
	svc := NewService(&projectRepositoryStub{project: domain.CodebookProject{
		ID: 7, Status: domain.CodebookProjectStatusArchived,
	}})

	_, err := svc.UpdateProject(t.Context(), domain.CodebookProject{ID: 7, Name: "剧本"})

	require.ErrorIs(t, err, errs.ErrInvalidParameter)
	require.ErrorContains(t, err, "只读不可修改")
}
