package dao

import (
	"strings"
	"testing"

	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/Duke1616/eiam/pkg/gormx"
	"github.com/Duke1616/etask/pkg/sqlx"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newTaskOverrideDryRunDB(t *testing.T) (*gorm.DB, *sqlRecorder) {
	t.Helper()
	recorder := &sqlRecorder{Interface: logger.Default.LogMode(logger.Info)}
	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN:                       "gorm:gorm@tcp(localhost:9910)/gorm?charset=utf8mb4&parseTime=True&loc=Local",
		SkipInitializeWithVersion: true,
	}), &gorm.Config{
		DryRun:                 true,
		DisableAutomaticPing:   true,
		SkipDefaultTransaction: true,
		Logger:                 recorder,
	})
	require.NoError(t, err)
	require.NoError(t, db.Use(gormx.NewTenantPlugin()))
	return db, recorder
}

func TestTaskOverrideQueriesUseTenantPlugin(t *testing.T) {
	db, recorder := newTaskOverrideDryRunDB(t)
	dao := &GORMTaskDAO{db: db}
	ctx := ctxutil.WithTenantID(t.Context(), 9)
	task := Task{ID: 7}

	require.NoError(t, dao.loadOverrideRules(ctx, &task))
	require.Contains(t, recorder.statement[strings.Index(recorder.statement, "WHERE"):], "tenant_id")

	require.NoError(t, dao.loadPendingParamOverrides(ctx, &task))
	require.Contains(t, recorder.statement[strings.Index(recorder.statement, "WHERE"):], "tenant_id")
}

func TestTaskRunOverrideCreateFillsTenantID(t *testing.T) {
	db, _ := newTaskOverrideDryRunDB(t)
	pending := TaskRunParamOverride{
		TaskID: 7,
		Overrides: sqlx.JSONColumn[map[string]string]{
			Val:   map[string]string{"limit": "host01"},
			Valid: true,
		},
	}

	err := db.WithContext(ctxutil.WithTenantID(t.Context(), 9)).Create(&pending).Error

	require.NoError(t, err)
	require.Equal(t, int64(9), pending.TenantID)
}
