package dao

import (
	"testing"

	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/Duke1616/eiam/pkg/gormx"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestProjectDeleteImpact(t *testing.T) {
	snapshot := projectDeletionSnapshot{
		project:           CodebookProject{ID: 7, Name: "剧本"},
		nodeIDs:           []int64{1, 2},
		versionCount:      2,
		versionObjectKeys: []string{"codebook-content/1/a"},
		artifactReleases:  []ArtifactRelease{{ID: 5, Size: 10}},
		projectSources:    []ProjectSource{{ID: 6, Size: 20}},
		taskIDs:           []int64{8}, activeTaskCount: 1,
		conversationIDs:    []int64{9},
		retainedReleaseIDs: map[int64]struct{}{5: {}},
		retainedSourceIDs:  map[int64]struct{}{6: {}},
	}

	impact := snapshot.impact()

	require.Equal(t, int64(2), impact.CodebookNodeCount)
	require.Equal(t, int64(2), impact.CodebookVersionCount)
	require.Equal(t, int64(10), impact.ArtifactReleaseBytes)
	require.Equal(t, int64(20), impact.ProjectSourceBytes)
	require.Equal(t, int64(1), impact.ActiveTaskCount)
	require.Equal(t, int64(1), impact.RetainedArtifactReleaseCount)
	require.Equal(t, int64(1), impact.RetainedProjectSourceCount)
}

func TestDeletionReferencesOnlyKeepProjectRows(t *testing.T) {
	releases := []ArtifactRelease{{ID: 1}, {ID: 2}}
	sources := []ProjectSource{{ID: 3}, {ID: 4}}

	retainedReleases := intersectArtifactReleaseIDs(releases, map[int64]struct{}{2: {}, 99: {}})
	retainedSources := intersectProjectSourceIDs(sources, map[int64]struct{}{4: {}, 99: {}})

	require.Equal(t, map[int64]struct{}{2: {}}, retainedReleases)
	require.Equal(t, map[int64]struct{}{4: {}}, retainedSources)
}

func TestRetainedProjectObjectKeysProtectSharedReleaseObject(t *testing.T) {
	snapshot := projectDeletionSnapshot{
		artifactReleases: []ArtifactRelease{
			{ID: 1, ObjectKey: "shared"},
			{ID: 2, ObjectKey: "shared"},
		},
		projectSources:     []ProjectSource{{ID: 3, ObjectKey: "source"}},
		retainedReleaseIDs: map[int64]struct{}{1: {}},
		retainedSourceIDs:  map[int64]struct{}{3: {}},
	}

	keys := retainedProjectObjectKeys(snapshot)

	require.Contains(t, keys, "shared")
	require.Contains(t, keys, "source")
}

func TestProjectDeletionQueriesUseTenantPlugin(t *testing.T) {
	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN:                       "gorm:gorm@tcp(localhost:9910)/gorm?charset=utf8mb4&parseTime=True&loc=Local",
		SkipInitializeWithVersion: true,
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	require.NoError(t, err)
	require.NoError(t, db.Use(gormx.NewTenantPlugin()))

	ctx := ctxutil.WithTenantID(t.Context(), 9)
	taskQuery := db.WithContext(ctx).Model(&Task{}).
		Select("id, status").
		Where(`CAST(COALESCE(
			program->>'$.project.entryCodebookId',
			program->>'$.inline.codebookId'
		) AS UNSIGNED) IN ?`, []int64{11}).
		Find(&[]Task{})
	executionQuery := db.WithContext(ctx).Model(&TaskExecution{}).
		Select("DISTINCT refs.release_id AS id").
		Joins(`JOIN JSON_TABLE(task_executions.artifact, '$[*]' COLUMNS (
			release_id BIGINT PATH '$.releaseId',
			project_id BIGINT PATH '$.projectId',
			scope VARCHAR(32) PATH '$.scope'
		)) AS refs ON TRUE`).
		Where("refs.scope = ? AND refs.project_id = ?", "TENANT", 7).
		Find(&[]struct{ ID int64 }{})

	require.Contains(t, taskQuery.Statement.SQL.String(), "tenant_id = ?")
	require.Contains(t, executionQuery.Statement.SQL.String(), "tenant_id = ?")
}
