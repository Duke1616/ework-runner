package ioc

import (
	"context"
	"database/sql"
	"log"
	"os"
	"time"

	"github.com/Duke1616/ework-runner/internal/repository/dao"
	"github.com/spf13/viper"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"

	"github.com/ecodeclub/ekit/retry"
)

func InitDB() *gorm.DB {
	dsn := viper.GetString("mysql.dsn")
	WaitForDBSetup(dsn)

	type DataBaseConnConfigurator struct {
		DBConnectString           string //连接字符串
		MaxIdleConns              int    //空闲连接池中连接的最大数量
		MaxOpenConns              int    //打开数据库连接的最大数量
		ConnMaxLifetime           int    //连接可复用的最大时间（分钟）
		SlowThreshold             int64  //慢SQL阈值(秒)
		LogLevel                  int    //日志级别 1:Silent  2:Error 3:Warn 4:Info
		IgnoreRecordNotFoundError bool   //忽略ErrRecordNotFound（记录未找到）错误
		Colorful                  bool   //使用彩色打印
	}
	var DBConnConfigurator = DataBaseConnConfigurator{
		MaxIdleConns: 10, MaxOpenConns: 100, ConnMaxLifetime: 3600, SlowThreshold: 1, LogLevel: 3,
		IgnoreRecordNotFoundError: true, Colorful: true}

	myLogger := logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags),
		logger.Config{
			SlowThreshold:             time.Duration(DBConnConfigurator.SlowThreshold) * time.Second, // 慢SQL阈值
			LogLevel:                  logger.LogLevel(DBConnConfigurator.LogLevel),                  // 日志级别
			IgnoreRecordNotFoundError: DBConnConfigurator.IgnoreRecordNotFoundError,                  // 忽略ErrRecordNotFound（记录未找到）错误
			Colorful:                  DBConnConfigurator.Colorful,                                   // 使用彩色打印
		},
	)

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			TablePrefix:   "",   //表前缀.如前缀为t_，则`User` 的表名应该是 `t_users`
			SingularTable: true, //使用单数表名，启用该选项，此时，`User` 的表名应该是 `user`
		},
		Logger: myLogger,
	})

	err = dao.InitTables(db)
	if err != nil {
		panic(err)
	}
	return db
}

func WaitForDBSetup(dsn string) {
	sqlDB, err := sql.Open("mysql", dsn)
	if err != nil {
		panic(err)
	}
	const maxInterval = 10 * time.Second
	const maxRetries = 10
	strategy, err := retry.NewExponentialBackoffRetryStrategy(time.Second, maxInterval, maxRetries)
	if err != nil {
		panic(err)
	}

	const timeout = 5 * time.Second
	for {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		err = sqlDB.PingContext(ctx)
		cancel()
		if err == nil {
			break
		}
		next, ok := strategy.Next()
		if !ok {
			panic("WaitForDBSetup 重试失败......")
		}
		time.Sleep(next)
	}
}
