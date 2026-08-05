package main

import (
	"context"
	"os"

	"github.com/Duke1616/etask/ioc"
	"github.com/fsnotify/fsnotify"
	"github.com/gotomicro/ego"
	"github.com/gotomicro/ego/core/elog"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

func main() {
	initViper()

	rootCmd := &cobra.Command{
		Use:   "scheduler",
		Short: "ework-runner scheduler",
		// 如果不输入子命令，就默认执行服务器
		Run: func(cmd *cobra.Command, args []string) {
			startServer()
		},
	}

	serverCmd := &cobra.Command{
		Use:   "server",
		Short: "启动调度器服务",
		Run: func(cmd *cobra.Command, args []string) {
			startServer()
		},
	}

	// 注册子命令
	rootCmd.AddCommand(serverCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func startServer() {
	// 1. 初始化共享基础设施
	base := ioc.InitBase()
	app := ioc.NewApp()

	// 2. 按模式加载所需模块
	if err := app.LoadByModes(base, []string{ioc.ModeScheduler}); err != nil {
		elog.Panic("load_mode_error", elog.FieldErr(err))
	}

	// 3. 启动后台任务和服务
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	app.StartBackgroundTasks(ctx)

	if err := ego.New().Serve(app.GetServers()...).Run(); err != nil {
		elog.Panic("startup", elog.FieldErr(err))
	}
}

func initViper() {
	dir, _ := os.Getwd()

	file := pflag.String(
		"config",
		dir+"/../../config/config.yaml",
		"配置文件路径",
	)
	pflag.Parse()

	viper.SetConfigFile(*file)

	viper.WatchConfig()
	if err := viper.ReadInConfig(); err != nil {
		panic(err)
	}

	setLogLevel()

	viper.OnConfigChange(func(in fsnotify.Event) {
		setLogLevel()
	})
}

// setLogLevel 根据配置文件中的 log.debug 动态调整全局日志级别
func setLogLevel() {
	if viper.GetBool("log.debug") {
		elog.DefaultLogger.SetLevel(elog.DebugLevel)
		elog.DefaultLogger.Debug("已根据配置开启 Debug 日志级别")
	} else {
		elog.DefaultLogger.SetLevel(elog.InfoLevel)
	}
}
