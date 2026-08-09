package dao

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/Duke1616/eiam/pkg/gormx"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type sqlRecorder struct {
	logger.Interface
	statement string
}

func (r *sqlRecorder) Trace(_ context.Context, _ time.Time,
	fc func() (string, int64), _ error) {
	r.statement, _ = fc()
}

func TestCodebookProjectGetByIDIncludesArchivedProjects(t *testing.T) {
	recorder := &sqlRecorder{Interface: logger.Default.LogMode(logger.Info)}
	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN:                       "gorm:gorm@tcp(localhost:9910)/gorm?charset=utf8mb4&parseTime=True&loc=Local",
		SkipInitializeWithVersion: true,
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true, Logger: recorder})
	require.NoError(t, err)
	require.NoError(t, db.Use(gormx.NewTenantPlugin()))

	_, err = NewGORMCodebookProjectDAO(db).GetByID(ctxutil.WithTenantID(t.Context(), 9), 7)

	require.NoError(t, err)
	whereClause := recorder.statement[strings.Index(recorder.statement, "WHERE"):]
	require.Contains(t, whereClause, "tenant_id")
	require.NotContains(t, whereClause, "status")
}

func TestReferenceProjectsExcludePinnedProject(t *testing.T) {
	recorder := &sqlRecorder{Interface: logger.Default.LogMode(logger.Info)}
	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN:                       "gorm:gorm@tcp(localhost:9910)/gorm?charset=utf8mb4&parseTime=True&loc=Local",
		SkipInitializeWithVersion: true,
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true, Logger: recorder})
	require.NoError(t, err)
	require.NoError(t, db.Use(gormx.NewTenantPlugin()))
	dao := NewGORMCodebookProjectDAO(db)
	ctx := ctxutil.WithTenantID(t.Context(), 9)

	_, err = dao.ListReferenceProjects(ctx, "", 7, 0, 20)
	require.NoError(t, err)
	require.Contains(t, recorder.statement, "<> 7")

	_, err = dao.CountReferenceProjects(ctx, "", 7)
	require.NoError(t, err)
	require.Contains(t, recorder.statement, "<> 7")
}
