package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/Duke1616/ework-runner/internal/domain"
	"github.com/Duke1616/ework-runner/internal/repository/dao"
	"github.com/Duke1616/ework-runner/ioc"
	"github.com/Duke1616/ework-runner/pkg/sqlx"
	"github.com/google/uuid"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

// 这个方法用于初始化任务,需要提前将执行节点，调度节点启动
func TestStart(t *testing.T) {
	initViper()

	// 初始化db
	db := ioc.InitDB()
	taskDAO := dao.NewGORMTaskDAO(db)
	// 初始化task
	now := time.Now()
	taskName := fmt.Sprintf("task_%s", uuid.New().String())
	_, err := taskDAO.Create(t.Context(), dao.Task{
		Name:     taskName,
		CronExpr: "0 0 * * * ?",
		GrpcConfig: sqlx.JSONColumn[domain.GrpcConfig]{
			Valid: true,
			Val: domain.GrpcConfig{
				ServiceName: "builder",
			},
		},
		ScheduleParams: sqlx.JSONColumn[map[string]string]{
			Valid: true,
			Val: map[string]string{
				"start": "0",
				"end":   "100",
			},
		},
		Status:   domain.TaskStatusActive.String(),
		Version:  1,
		NextTime: now.Add(3 * time.Second).UnixMilli(),
	})
	require.NoError(t, err)
}

func initViper() {
	file := pflag.String("config",
		"/Users/luankz/go-code/ework-runner/config/config.yaml", "配置文件路径")
	pflag.Parse()
	// 直接指定文件路径
	viper.SetConfigFile(*file)
	viper.WatchConfig()
	err := viper.ReadInConfig()

	if err != nil {
		panic(err)
	}
}
