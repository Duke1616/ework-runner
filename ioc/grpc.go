package ioc

import (
	"fmt"
	"time"

	executorv1 "github.com/Duke1616/ework-runner/api/proto/gen/executor/v1"
	reporterv1 "github.com/Duke1616/ework-runner/api/proto/gen/reporter/v1"
	grpcapi "github.com/Duke1616/ework-runner/internal/grpc"
	grpcpkg "github.com/Duke1616/ework-runner/pkg/grpc"
	registrysdk "github.com/Duke1616/ework-runner/pkg/grpc/registry"
	"github.com/spf13/viper"
	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc"
)

// InitSchedulerNodeGRPCServer 初始化 Scheduler gRPC 服务器
func InitSchedulerNodeGRPCServer(etcdClient *clientv3.Client, reporter *grpcapi.ReporterServer) *grpcpkg.Server {
	var cfg ServerConfig
	if err := viper.UnmarshalKey("server.scheduler.grpc", &cfg); err != nil {
		panic(err)
	}

	server := grpcpkg.NewServer(cfg.Name, cfg.Addr(), etcdClient)
	reporterv1.RegisterReporterServiceServer(server.Server, reporter)

	return server
}

func InitExecutorServiceGRPCClients(registry registrysdk.Registry) *grpcpkg.Clients[executorv1.ExecutorServiceClient] {
	const defaultTimeout = time.Second
	return grpcpkg.NewClients(
		registry,
		defaultTimeout,
		func(conn *grpc.ClientConn) executorv1.ExecutorServiceClient {
			return executorv1.NewExecutorServiceClient(conn)
		})
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
