package ioc

import (
	executorv1 "github.com/Duke1616/ecmdb/api/proto/gen/executor/v1"
	"github.com/Duke1616/ecmdb/internal/compensator"
	"github.com/Duke1616/ecmdb/internal/service/runner"
	"github.com/Duke1616/ecmdb/internal/service/task"
	"github.com/Duke1616/ecmdb/pkg/grpc"
	"github.com/gotomicro/ego/core/econf"
)

func InitRetryCompensator(
	runner runner.Runner,
	execSvc task.ExecutionService,
) *compensator.RetryCompensator {
	var cfg compensator.RetryConfig
	err := econf.UnmarshalKey("compensator.retry", &cfg)
	if err != nil {
		panic(err)
	}
	return compensator.NewRetryCompensator(
		runner,
		execSvc,
		cfg,
	)
}

func InitRescheduleCompensator(
	runner runner.Runner,
	execSvc task.ExecutionService,
) *compensator.RescheduleCompensator {
	var cfg compensator.RescheduleConfig
	err := econf.UnmarshalKey("compensator.reschedule", &cfg)
	if err != nil {
		panic(err)
	}
	return compensator.NewRescheduleCompensator(
		runner,
		execSvc,
		cfg)
}

func InitInterruptCompensator(
	grpcClients *grpc.Clients[executorv1.ExecutorServiceClient],
	execSvc task.ExecutionService,
) *compensator.InterruptCompensator {
	var cfg compensator.InterruptConfig
	err := econf.UnmarshalKey("compensator.interrupt", &cfg)
	if err != nil {
		panic(err)
	}
	return compensator.NewInterruptCompensator(
		grpcClients,
		execSvc,
		cfg,
	)
}
