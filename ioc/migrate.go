package ioc

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Duke1616/etask/deploy/migrations"
	"github.com/Duke1616/etask/internal/repository/dao"
	"github.com/gotomicro/ego/core/elog"
	"github.com/pressly/goose/v3"
	"gorm.io/gorm"
)

const (
	migrationLockName           = "etask_schema_migration"
	migrationLockTimeoutSeconds = 300
)

// RunMigrations 在应用启动时初始化表并执行待执行的 SQL 迁移。
// MySQL advisory lock 保证多个实例启动时不会并发修改数据库结构。
func RunMigrations(db *gorm.DB) {
	sqlDB, err := db.DB()
	if err != nil {
		panic("获取 *sql.DB 失败: " + err.Error())
	}

	ctx := context.Background()
	err = withMigrationLock(ctx, sqlDB, func() error {
		// AutoMigrate 负责基础表和新增字段，复杂变更继续由 Goose 管理。
		if migrateErr := dao.InitTables(db); migrateErr != nil {
			return fmt.Errorf("初始化数据库表失败: %w", migrateErr)
		}
		if migrateErr := runGooseMigrations(ctx, sqlDB); migrateErr != nil {
			return fmt.Errorf("执行 Goose 迁移失败: %w", migrateErr)
		}
		return nil
	})
	if err != nil {
		panic("数据库迁移失败: " + err.Error())
	}

	elog.DefaultLogger.Info("数据库迁移完成")
}

func withMigrationLock(ctx context.Context, sqlDB *sql.DB, migrate func() error) error {
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return fmt.Errorf("获取迁移锁连接失败: %w", err)
	}
	defer conn.Close()

	var acquired sql.NullInt64
	err = conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, ?)",
		migrationLockName, migrationLockTimeoutSeconds).Scan(&acquired)
	if err != nil {
		return fmt.Errorf("获取数据库迁移锁失败: %w", err)
	}
	if !acquired.Valid || acquired.Int64 != 1 {
		return fmt.Errorf("等待数据库迁移锁超时: %s", migrationLockName)
	}
	defer releaseMigrationLock(conn)

	return migrate()
}

func releaseMigrationLock(conn *sql.Conn) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var released sql.NullInt64
	if err := conn.QueryRowContext(ctx, "SELECT RELEASE_LOCK(?)", migrationLockName).Scan(&released); err != nil {
		elog.DefaultLogger.Error("释放数据库迁移锁失败", elog.FieldErr(err))
	}
}

func runGooseMigrations(ctx context.Context, sqlDB *sql.DB) error {
	provider, err := goose.NewProvider(
		goose.DialectMySQL,
		sqlDB,
		migrations.FS,
		goose.WithDisableGlobalRegistry(true),
	)
	if err != nil {
		return err
	}
	_, err = provider.Up(ctx)
	return err
}
