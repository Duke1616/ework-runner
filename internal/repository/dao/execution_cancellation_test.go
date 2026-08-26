package dao

import (
	"testing"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestReleaseTaskOnCancellationUsesExecutionScheduleOwner(t *testing.T) {
	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN:                       "gorm:gorm@tcp(localhost:9910)/gorm?charset=utf8mb4&parseTime=True&loc=Local",
		SkipInitializeWithVersion: true,
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	require.NoError(t, err)

	sql := db.ToSQL(func(tx *gorm.DB) *gorm.DB {
		_ = releaseTaskOnCancellation(tx, TaskExecution{
			TaskID: 42, TaskType: string(domain.TaskTypeRecurring), TaskScheduleNodeID: "scheduler-a",
		})
		return tx
	})

	require.Contains(t, sql, "WHERE id = 42 AND status = 'PREEMPTED' AND schedule_node_id = 'scheduler-a'")
	require.Contains(t, sql, "status`='ACTIVE'")
}

func TestReleaseTaskOnCancellationCompletesOneTimeTask(t *testing.T) {
	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN:                       "gorm:gorm@tcp(localhost:9910)/gorm?charset=utf8mb4&parseTime=True&loc=Local",
		SkipInitializeWithVersion: true,
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	require.NoError(t, err)

	sql := db.ToSQL(func(tx *gorm.DB) *gorm.DB {
		_ = releaseTaskOnCancellation(tx, TaskExecution{
			TaskID: 42, TaskType: string(domain.TaskTypeOneTime), TaskScheduleNodeID: "scheduler-a",
		})
		return tx
	})

	require.Contains(t, sql, "status`='COMPLETED'")
}
