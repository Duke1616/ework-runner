package ioc

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"time"

	"github.com/Duke1616/etask/deploy/migrations"
	"github.com/Duke1616/etask/internal/repository/dao"
	"github.com/gotomicro/ego/core/elog"
	"github.com/pressly/goose/v3"
	"gorm.io/gorm"
)

const (
	migrationLockName          = "etask_schema_migration"
	migrationLockWaitTimeout   = 5 * time.Minute
	migrationLockRetryInterval = time.Second
)

var errMigrationLockTimeout = errors.New("等待数据库迁移锁超时")

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

	attempts := 0
	err = waitForMigrationLock(ctx, migrationLockWaitTimeout, migrationLockRetryInterval,
		func(ctx context.Context) (bool, error) {
			acquired, acquireErr := tryAcquireMigrationLock(ctx, conn)
			if acquireErr != nil || acquired {
				return acquired, acquireErr
			}
			attempts++
			if attempts == 1 || attempts%30 == 0 {
				logMigrationLockWait(ctx, conn)
			}
			return false, nil
		})
	if errors.Is(err, errMigrationLockTimeout) {
		return migrationLockTimeoutError(conn)
	}
	if err != nil {
		return fmt.Errorf("获取数据库迁移锁失败: %w", err)
	}
	defer releaseMigrationLock(conn)

	return migrate()
}

func waitForMigrationLock(ctx context.Context, timeout, retryInterval time.Duration,
	tryAcquire func(context.Context) (bool, error)) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	ticker := time.NewTicker(retryInterval)
	defer ticker.Stop()

	for {
		acquired, err := tryAcquire(ctx)
		if err != nil {
			return err
		}
		if acquired {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return errMigrationLockTimeout
		case <-ticker.C:
		}
	}
}

func tryAcquireMigrationLock(ctx context.Context, conn *sql.Conn) (bool, error) {
	var acquired sql.NullInt64
	if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, 0)", migrationLockName).Scan(&acquired); err != nil {
		return false, err
	}
	if !acquired.Valid {
		return false, fmt.Errorf("GET_LOCK 返回 NULL")
	}
	return acquired.Int64 == 1, nil
}

func logMigrationLockWait(ctx context.Context, conn *sql.Conn) {
	var owner sql.NullInt64
	err := conn.QueryRowContext(ctx, "SELECT IS_USED_LOCK(?)", migrationLockName).Scan(&owner)
	if err != nil {
		elog.DefaultLogger.Warn("正在等待数据库迁移锁",
			elog.String("lockName", migrationLockName), elog.FieldErr(err))
		return
	}
	if owner.Valid {
		elog.DefaultLogger.Warn("正在等待数据库迁移锁",
			elog.String("lockName", migrationLockName), elog.Int64("ownerConnectionID", owner.Int64))
		return
	}
	elog.DefaultLogger.Warn("正在等待数据库迁移锁",
		elog.String("lockName", migrationLockName))
}

func migrationLockTimeoutError(conn *sql.Conn) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var owner sql.NullInt64
	if err := conn.QueryRowContext(ctx, "SELECT IS_USED_LOCK(?)", migrationLockName).Scan(&owner); err != nil {
		return fmt.Errorf("%w: %s（查询持锁连接失败: %v）", errMigrationLockTimeout, migrationLockName, err)
	}
	if owner.Valid {
		return fmt.Errorf("%w: %s（持锁连接 ID: %d）", errMigrationLockTimeout, migrationLockName, owner.Int64)
	}
	return fmt.Errorf("%w: %s", errMigrationLockTimeout, migrationLockName)
}

func releaseMigrationLock(conn *sql.Conn) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var released sql.NullInt64
	if err := conn.QueryRowContext(ctx, "SELECT RELEASE_LOCK(?)", migrationLockName).Scan(&released); err != nil {
		elog.DefaultLogger.Error("释放数据库迁移锁失败", elog.FieldErr(err))
		discardConnection(conn)
		return
	}
	if !released.Valid || released.Int64 != 1 {
		elog.DefaultLogger.Warn("数据库迁移锁未由当前连接持有",
			elog.String("lockName", migrationLockName))
	}
}

// 锁释放失败时不能将底层连接放回连接池，否则 MySQL 命名锁会继续存活。
func discardConnection(conn *sql.Conn) {
	err := conn.Raw(func(any) error { return driver.ErrBadConn })
	if err != nil && !errors.Is(err, driver.ErrBadConn) {
		elog.DefaultLogger.Error("销毁迁移锁连接失败", elog.FieldErr(err))
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
