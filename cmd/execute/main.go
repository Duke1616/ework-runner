package main

import (
	"github.com/Duke1616/ework-runner/internal/grpc"
	"github.com/Duke1616/ework-runner/sdk/executor"
	"github.com/gotomicro/ego/core/elog"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

func main() {
	initViper()

	// 读取配置
	cfg := &executor.Config{
		NodeID:              viper.GetString("server.executor.grpc.id"),
		ServiceName:         viper.GetString("server.executor.grpc.name"),
		Addr:                viper.GetString("server.executor.grpc.host") + ":" + viper.GetString("server.executor.grpc.port"),
		EtcdEndpoints:       viper.GetStringSlice("etcd.endpoints"),
		ReporterServiceName: "scheduler", // 使用服务发现
		// ReporterAddr:        "198.18.0.1:9002", // 可选:直接地址
	}

	// 创建 Executor 并注册处理函数
	exec := executor.MustNewExecutor(cfg)
	exec.RegisterHandler(grpc.DemoTaskHandler)

	// 启动并阻塞
	if err := exec.Start(); err != nil {
		elog.Panic("启动失败", elog.FieldErr(err))
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
