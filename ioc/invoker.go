package ioc

import (
	executorv1 "github.com/Duke1616/ework-runner/api/proto/gen/executor/v1"
	"github.com/Duke1616/ework-runner/internal/service/invoker"
	"github.com/Duke1616/ework-runner/pkg/grpc"
)

func InitInvoker(clients *grpc.Clients[executorv1.ExecutorServiceClient]) invoker.Invoker {
	return invoker.NewDispatcher(
		invoker.NewHTTPInvoker(),
		invoker.NewGRPCInvoker(clients),
		invoker.NewLocalInvoker(map[string]invoker.LocalExecuteFunc{}, map[string]invoker.LocalPrepareFunc{}))
}
