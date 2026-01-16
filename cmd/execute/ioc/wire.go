//go:build wireinject

package ioc

import (
	"fmt"

	executorv1 "github.com/Duke1616/ework-runner/api/proto/gen/executor/v1"
	reporterv1 "github.com/Duke1616/ework-runner/api/proto/gen/reporter/v1"
	grpcapi "github.com/Duke1616/ework-runner/internal/grpc"
	"github.com/Duke1616/ework-runner/ioc"
	"github.com/Duke1616/ework-runner/pkg/grpc/registry"
	"github.com/Duke1616/ework-runner/pkg/grpc/registry/etcd"
	"github.com/Duke1616/ework-runner/sdk/executor"
	"github.com/google/wire"
	"github.com/spf13/viper"
	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	BaseSet = wire.NewSet(
		ioc.InitEtcdClient,
		InitRegistry,
	)

	grpcServerSet = wire.NewSet(
		InitExecutorGRPCServer,
	)

	grpcClientSet = wire.NewSet(
		initReporterServiceClient,
	)
)

func InitExecuteApp() *ExecuteApp {
	wire.Build(
		// 基础设施
		BaseSet,

		// gRPC 服务器
		grpcServerSet,

		// gRPC 客户端
		grpcClientSet,

		wire.Struct(new(ExecuteApp), "*"),
	)

	return new(ExecuteApp)
}

// InitRegistry 初始化注册中心
func InitRegistry(client *clientv3.Client) registry.Registry {
	reg, err := etcd.NewRegistry(client)
	if err != nil {
		panic(err)
	}
	return reg
}

func initReporterServiceClient() reporterv1.ReporterServiceClient {
	// 直接使用 IP:Port 地址
	target := fmt.Sprintf("%s:%d", "127.0.0.1", 9002)

	conn, err := grpc.NewClient(
		target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)

	if err != nil {
		panic(err)
	}
	return reporterv1.NewReporterServiceClient(conn)
}

// InitExecutorGRPCServer 初始化 Executor gRPC 服务器
func InitExecutorGRPCServer(reg registry.Registry, client reporterv1.ReporterServiceClient) *executor.Server {
	var cfg ServerConfig
	if err := viper.UnmarshalKey("server.executor.grpc", &cfg); err != nil {
		panic(err)
	}

	server := executor.NewServer(cfg.Id, cfg.Name, cfg.Addr(), reg)

	// 创建并注册 Executor 服务
	exec := grpcapi.NewExecutor(client)
	executorv1.RegisterExecutorServiceServer(server.Server, exec)

	return server
}

type ServerConfig struct {
	Id   string `mapstructure:"id"`
	Name string `mapstructure:"name"`
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
}

// Addr 返回服务器地址
func (c *ServerConfig) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

type ExecuteApp struct {
	Server *executor.Server
}
