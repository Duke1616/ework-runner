package ioc

import (
	"fmt"
	"time"

	executorv1 "github.com/Duke1616/ework-runner/api/proto/gen/executor/v1"
	reporterv1 "github.com/Duke1616/ework-runner/api/proto/gen/reporter/v1"
	grpcapi "github.com/Duke1616/ework-runner/internal/grpc"
	grpcpkg "github.com/Duke1616/ework-runner/pkg/grpc"
	registrysdk "github.com/Duke1616/ework-runner/pkg/grpc/registry"
	"github.com/Duke1616/ework-runner/pkg/grpc/registry/etcd"
	"github.com/spf13/viper"
	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc"
)

// InitSchedulerNodeGRPCServer 初始化 Scheduler gRPC 服务器
func InitSchedulerNodeGRPCServer(registry registrysdk.Registry, reporter *grpcapi.ReporterServer) *grpcpkg.Server {
	var cfg ServerConfig
	if err := viper.UnmarshalKey("server.scheduler.grpc", &cfg); err != nil {
		panic(err)
	}

	server := grpcpkg.NewServer(cfg.Id, cfg.Name, cfg.Addr(), registry)
	reporterv1.RegisterReporterServiceServer(server.Server, reporter)

	return server
}

func InitExecutorServiceGRPCClients(etcdClient *clientv3.Client) *grpcpkg.Clients[executorv1.ExecutorServiceClient] {
	// NOTE: gRPC Clients 需要使用 Executor 的注册前缀来查找节点
	// Executor 注册在 /services/etask/executor 前缀下
	executorReg, err := etcd.NewRegistry(etcdClient)
	if err != nil {
		panic(err)
	}

	const defaultTimeout = time.Second
	return grpcpkg.NewClients(
		executorReg,
		defaultTimeout,
		func(conn *grpc.ClientConn) executorv1.ExecutorServiceClient {
			return executorv1.NewExecutorServiceClient(conn)
		})
}

type ServerConfig struct {
	Id            string `mapstructure:"id"`
	Name          string `mapstructure:"name"`
	Host          string `mapstructure:"host"`
	Port          int    `mapstructure:"port"`
	AdvertiseAddr string `mapstructure:"advertise_addr"` // 可选:手动指定注册到etcd的IP
}

// Addr 返回服务器地址
func (c *ServerConfig) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}
