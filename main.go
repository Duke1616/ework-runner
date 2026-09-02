package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/Duke1616/eiam/pkg/gormx"
	codebookcmd "github.com/Duke1616/etask/cmd/codebook"
	"github.com/Duke1616/etask/cmd/migrate"
	"github.com/Duke1616/etask/ioc"
	"github.com/fsnotify/fsnotify"
	"github.com/gotomicro/ego"
	"github.com/gotomicro/ego/core/elog"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	modes   []string
	cfgFile string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "ework-runner",
		Short: "ework-runner 统一入口",
	}

	dir, _ := os.Getwd()
	defaultCfg := dir + "/config/config.yaml"
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", defaultCfg, "配置文件路径")

	cobra.OnInitialize(initViper)

	serverCmd := &cobra.Command{
		Use:   "server",
		Short: "启动服务节点",
		Run: func(cmd *cobra.Command, args []string) {
			startServer()
		},
	}

	serverCmd.Flags().StringSliceVar(&modes, "mode", []string{"all"}, "启动模式 (all | scheduler | agent | executor)")

	rootCmd.AddCommand(serverCmd)
	rootCmd.AddCommand(codebookcmd.NewCommand())
	rootCmd.AddCommand(migrate.NewCommand())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func startServer() {
	// 1. 初始化所有模块共享的基础设施（仅连接，不启动业务）
	base := ioc.InitBase()
	app := ioc.NewApp()

	// 2. 按固定顺序加载所需模块，并拒绝未知模式。
	if err := app.LoadByModes(base, modes); err != nil {
		elog.Panic("load_mode_error", elog.FieldErr(err))
	}

	// 3. 启动已加载模块的服务和后台任务
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	app.StartBackgroundTasks(gormx.IgnoreTenantContext(ctx))

	if err := ego.New().Serve(app.GetServers()...).Run(); err != nil {
		// Ego 使用 context deadline 表示关闭窗口耗尽；任务状态会由调度补偿器继续收敛，
		// 不应把这个预期的停止结果再次升级为 panic。
		if errors.Is(err, context.DeadlineExceeded) {
			elog.Warn("应用优雅停止超时，剩余任务交由补偿器处理", elog.FieldErr(err))
			return
		}
		elog.Panic("app_run_error", elog.FieldErr(err))
	}
}

func initViper() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	}

	viper.WatchConfig()
	if err := viper.ReadInConfig(); err != nil {
		fmt.Printf("Warning: 配置文件读取失败: %v\n", err)
	} else {
		fmt.Printf("Using config file: %s\n", viper.ConfigFileUsed())
		setLogLevel()
	}

	// 监听配置变更，支持动态切换日志级别
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
