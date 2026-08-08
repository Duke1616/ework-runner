package dao

import (
	"errors"
	"testing"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/errs"
	"github.com/Duke1616/etask/pkg/sorter"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestBuildCodebookImportPlanReusesDirectories(t *testing.T) {
	existing := []Codebook{
		{
			ID: 10, Scope: domain.CodebookScopeTenant.String(), ProjectID: 3,
			ParentID: 0, PathIDs: domain.CodebookRootPathIDs, Depth: 0,
			Name: "roles", Kind: domain.CodebookKindDirectory.String(), SortNo: 2000,
		},
		{
			ID: 11, Scope: domain.CodebookScopeTenant.String(), ProjectID: 3,
			ParentID: 10, PathIDs: "/10/", Depth: 1,
			Name: "README.md", Kind: domain.CodebookKindFile.String(), SortNo: 4000,
		},
	}
	plan, err := buildCodebookImportPlan(CodebookImport{
		ProjectID: 3,
		Files: []CodebookImportFile{
			{Path: "roles/web/tasks/main.yml"},
			{Path: "roles/api/tasks/main.yml"},
		},
	}, existing, 123)

	require.NoError(t, err)
	require.Equal(t, CodebookImportResult{FileCount: 2, DirectoryCount: 4}, plan.result())
	require.Len(t, plan.directories, 3)
	require.Empty(t, plan.directories[0]) // roles 已存在，不重复规划。
	require.Len(t, plan.directories[1], 2)
	require.Len(t, plan.directories[2], 2)
	require.Equal(t, int64(5000), plan.directories[1][0].entity.SortNo)
	require.Equal(t, int64(6000), plan.directories[1][1].entity.SortNo)
	require.Equal(t, int64(sorter.DefaultIndexGap), plan.directories[2][0].entity.SortNo)
}

func TestBuildCodebookImportPlanUsesRequestedParent(t *testing.T) {
	existing := []Codebook{{
		ID: 10, Scope: domain.CodebookScopeTenant.String(), ProjectID: 3,
		PathIDs: domain.CodebookRootPathIDs, Depth: 0,
		Name: "roles", Kind: domain.CodebookKindDirectory.String(), SortNo: 2000,
	}}
	plan, err := buildCodebookImportPlan(CodebookImport{
		ProjectID: 3, ParentID: 10,
		Files: []CodebookImportFile{{Path: "web/main.yml"}},
	}, existing, 123)

	require.NoError(t, err)
	require.Equal(t, int64(10), plan.directories[0][0].parent.entity.ID)
	require.Equal(t, "web", plan.directories[0][0].entity.Name)
	require.Equal(t, "main.yml", plan.files[0].entity.Name)
}

func TestBuildCodebookImportPlanRejectsConflicts(t *testing.T) {
	existing := []Codebook{{
		ID: 10, Scope: domain.CodebookScopeTenant.String(), ProjectID: 3,
		PathIDs: domain.CodebookRootPathIDs, Depth: 0,
		Name: "roles", Kind: domain.CodebookKindFile.String(),
	}}
	_, err := buildCodebookImportPlan(CodebookImport{
		ProjectID: 3, Files: []CodebookImportFile{{Path: "ROLES/main.yml"}},
	}, existing, 123)

	require.ErrorIs(t, err, errs.ErrCodebookNameConflict)
}

func TestBuildCodebookImportPlanRejectsInvalidParent(t *testing.T) {
	_, err := buildCodebookImportPlan(CodebookImport{
		ProjectID: 3, ParentID: 99, Files: []CodebookImportFile{{Path: "main.yml"}},
	}, nil, 123)

	require.True(t, errors.Is(err, gorm.ErrRecordNotFound))
}
