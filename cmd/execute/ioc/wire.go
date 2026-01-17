//go:build wireinject

package ioc

import (
	"github.com/Duke1616/ework-runner/internal/grpc"
	"github.com/Duke1616/ework-runner/internal/grpc/scripts"
	"github.com/Duke1616/ework-runner/ioc"
	grpcpkg "github.com/Duke1616/ework-runner/pkg/grpc"
	"github.com/Duke1616/ework-runner/pkg/grpc/registry"
	"github.com/Duke1616/ework-runner/pkg/grpc/registry/etcd"
	"github.com/Duke1616/ework-runner/pkg/netx"
	"github.com/Duke1616/ework-runner/sdk/executor"
	"github.com/google/wire"
	"github.com/spf13/viper"
	clientv3 "go.etcd.io/etcd/client/v3"
)

var (
	BaseSet = wire.NewSet(
		ioc.InitEtcdClient,
		InitRegistry,
	)

	ExecutorSet = wire.NewSet(
		InitConfig,
		InitExecutor,
		InitExecutorServer,
	)
)

func InitExecuteApp() *ExecuteApp {
	wire.Build(
		// 基础设施
		BaseSet,
		// Executor 组件
		ExecutorSet,
		wire.Struct(new(ExecuteApp), "*"),
	)

	return new(ExecuteApp)
}

// InitRegistry 初始化注册中心
func InitRegistry(client *clientv3.Client) registry.Registry {
	// NOTE: 统一使用 service 前缀
	reg, err := etcd.NewRegistryWithPrefix(client, "service")
	if err != nil {
		panic(err)
	}
	return reg
}

// InitConfig 初始化配置
func InitConfig() *executor.Config {
	host := viper.GetString("server.executor.grpc.host")
	if host == "0.0.0.0" || host == "" {
		host = netx.GetOutboundIP()
	}

	return &executor.Config{
		NodeID:              viper.GetString("server.executor.grpc.id"),
		ServiceName:         viper.GetString("server.executor.grpc.name"),
		Addr:                host + ":" + viper.GetString("server.executor.grpc.port"),
		ReporterServiceName: "scheduler",
	}
}

// InitExecutor 初始化 SDK Executor 实例
func InitExecutor(cfg *executor.Config, reg registry.Registry) *executor.Executor {
	exec, err := executor.NewExecutor(cfg, reg)
	if err != nil {
		panic(err)
	}

	// 注册处理函数
	exec.RegisterHandler(&grpc.DemoTaskHandler{})
	exec.RegisterHandler(scripts.NewShellTaskHandler())
	exec.RegisterHandler(scripts.NewPythonTaskHandler())

	// 初始化内部组件(连接Reporter等)
	if err = exec.InitComponents(); err != nil {
		panic(err)
	}

	return exec
}

// InitExecutorServer 从 Executor 中提取 ego Server
func InitExecutorServer(exec *executor.Executor) *grpcpkg.Server {
	return exec.Server()
}

type ExecuteApp struct {
	Server *grpcpkg.Server
}
