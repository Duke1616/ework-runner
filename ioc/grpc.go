package ioc

import (
	"time"

	executorv1 "github.com/Duke1616/ework-runner/api/proto/gen/executor/v1"
	reporterv1 "github.com/Duke1616/ework-runner/api/proto/gen/reporter/v1"
	grpcapi "github.com/Duke1616/ework-runner/internal/grpc"
	grpcpkg "github.com/Duke1616/ework-runner/pkg/grpc"
	registrysdk "github.com/Duke1616/ework-runner/pkg/grpc/registry"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
)

// InitSchedulerNodeGRPCServer 初始化 Scheduler gRPC 服务器
func InitSchedulerNodeGRPCServer(registry registrysdk.Registry, reporter *grpcapi.ReporterServer) *grpcpkg.Server {
	var cfg grpcpkg.Config
	if err := viper.UnmarshalKey("grpc.server.scheduler", &cfg); err != nil {
		panic(err)
	}

	server := grpcpkg.NewServer(cfg, registry, grpcpkg.WithJWTAuth(cfg.AuthToken))
	reporterv1.RegisterReporterServiceServer(server.Server, reporter)

	return server
}

func InitExecutorServiceGRPCClients(reg registrysdk.Registry) *grpcpkg.Clients[executorv1.ExecutorServiceClient] {
	const defaultTimeout = time.Second
	return grpcpkg.NewClients(
		reg,
		defaultTimeout,
		func(conn *grpc.ClientConn) executorv1.ExecutorServiceClient {
			return executorv1.NewExecutorServiceClient(conn)
		})
}
