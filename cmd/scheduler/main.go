package main

import (
	"context"

	"github.com/Duke1616/ework-runner/cmd/scheduler/ioc"
	"github.com/gotomicro/ego"
	"github.com/gotomicro/ego/core/elog"
	"github.com/gotomicro/ego/server"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

func main() {
	initViper()

	// 创建 ego 应用实例
	egoApp := ego.New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	app := ioc.InitSchedulerApp()
	app.StartTasks(ctx)

	// 启动服务
	if err := egoApp.Serve(
		func() server.Server {
			return app.Server
		}(),
		func() server.Server {
			return app.Scheduler
		}(),
	).Cron().
		Run(); err != nil {
		elog.Panic("startup", elog.FieldErr(err))
	}
}

func initViper() {
	file := pflag.String("config",
		"../../config/config.yaml", "配置文件路径")
	pflag.Parse()
	// 直接指定文件路径
	viper.SetConfigFile(*file)
	viper.WatchConfig()
	err := viper.ReadInConfig()

	if err != nil {
		panic(err)
	}
}
