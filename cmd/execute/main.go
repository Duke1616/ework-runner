package main

import (
	"github.com/Duke1616/ework-runner/cmd/execute/ioc"
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

	// 初始化 Executor 应用
	app := ioc.InitExecuteApp()

	// 启动服务
	if err := egoApp.Serve(
		func() server.Server {
			return app.Server
		}(),
	).Run(); err != nil {
		elog.Panic("startup", elog.FieldErr(err))
	}
}

func initViper() {
	file := pflag.String("config",
		"../../config/config.yaml", "配置文件路径")
	pflag.Parse()

	viper.SetConfigFile(*file)
	viper.WatchConfig()

	if err := viper.ReadInConfig(); err != nil {
		panic(err)
	}
}
