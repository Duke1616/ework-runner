package ioc

import (
	"time"

	executorv1 "github.com/Duke1616/ecmdb/api/proto/gen/executor/v1"
	grpcpkg "github.com/Duke1616/ecmdb/pkg/grpc"
	balancerRegistry "github.com/Duke1616/ecmdb/pkg/grpc/registry/etcd"
	"google.golang.org/grpc"
)

func InitExecutorServiceGRPCClients(registry *balancerRegistry.Registry) *grpcpkg.Clients[executorv1.ExecutorServiceClient] {
	const defaultTimeout = time.Second
	return grpcpkg.NewClients(
		registry,
		defaultTimeout,
		func(conn *grpc.ClientConn) executorv1.ExecutorServiceClient {
			return executorv1.NewExecutorServiceClient(conn)
		})
}
