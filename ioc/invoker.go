package ioc

import (
	executorv1 "github.com/Duke1616/ecmdb/api/proto/gen/executor/v1"
	"github.com/Duke1616/ecmdb/internal/service/invoker"
	"github.com/Duke1616/ecmdb/pkg/grpc"
)

func InitInvoker(clients *grpc.Clients[executorv1.ExecutorServiceClient]) invoker.Invoker {
	return invoker.NewDispatcher(
		invoker.NewHTTPInvoker(),
		invoker.NewGRPCInvoker(clients),
		invoker.NewLocalInvoker(map[string]invoker.LocalExecuteFunc{}, map[string]invoker.LocalPrepareFunc{}))
}
