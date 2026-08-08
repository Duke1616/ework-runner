package dao

import (
	"testing"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/pkg/sqlx"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestWithExecutionStatusCAS(t *testing.T) {
	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN:                       "gorm:gorm@tcp(localhost:9910)/gorm?charset=utf8mb4&parseTime=True&loc=Local",
		SkipInitializeWithVersion: true,
	}), &gorm.Config{
		DryRun:               true,
		DisableAutomaticPing: true,
	})
	require.NoError(t, err)

	sql := db.ToSQL(func(tx *gorm.DB) *gorm.DB {
		return withExecutionStatusCAS(tx.Model(&TaskExecution{}), 42,
			[]string{TaskExecutionStatusPrepare, TaskExecutionStatusRunning}).
			Updates(map[string]any{"status": "SUCCESS"})
	})

	require.Contains(t, sql, "WHERE id = 42 AND status IN ('PREPARE','RUNNING')")
}

func TestTaskUpdateFieldsIncludesProgram(t *testing.T) {
	program := sqlx.JSONColumn[domain.ProgramSpec]{
		Valid: true,
		Val: domain.ProgramSpec{
			Kind: domain.ProgramProject,
			Project: &domain.ProjectProgramSpec{
				EntryCodebookID: 113,
			},
		},
	}

	fields := taskUpdateFields(Task{Program: program})

	require.Equal(t, program, fields["program"])
}

func TestTaskUpdateFieldsCanClearProgram(t *testing.T) {
	fields := taskUpdateFields(Task{})

	program, exists := fields["program"]
	require.True(t, exists)
	require.Equal(t, sqlx.JSONColumn[domain.ProgramSpec]{}, program)
}
