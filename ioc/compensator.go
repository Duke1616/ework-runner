package ioc

import (
	executorv1 "github.com/Duke1616/etask/api/proto/gen/etask/executor/v1"
	"github.com/Duke1616/etask/internal/compensator"
	"github.com/Duke1616/etask/internal/service/dispatcher"
	"github.com/Duke1616/etask/internal/service/task"
	"github.com/Duke1616/etask/internal/service/termination"
	config "github.com/Duke1616/etask/pkg/config"
	"github.com/Duke1616/etask/pkg/grpc/pool"
)

func InitRetryCompensator(
	dispatcher dispatcher.Dispatcher,
	execSvc task.ExecutionService,
) *compensator.RetryCompensator {
	var cfg compensator.RetryConfig
	err := config.UnmarshalKey("compensator.retry", &cfg)
	if err != nil {
		panic(err)
	}
	return compensator.NewRetryCompensator(
		dispatcher,
		execSvc,
		cfg,
	)
}

func InitRescheduleCompensator(
	dispatcher dispatcher.Dispatcher,
	execSvc task.ExecutionService,
) *compensator.RescheduleCompensator {
	var cfg compensator.RescheduleConfig
	err := config.UnmarshalKey("compensator.reschedule", &cfg)
	if err != nil {
		panic(err)
	}
	return compensator.NewRescheduleCompensator(
		dispatcher,
		execSvc,
		cfg)
}

func InitInterruptCompensator(
	grpcClients *pool.Clients[executorv1.ExecutorServiceClient],
	execSvc task.ExecutionService,
) *compensator.InterruptCompensator {
	var cfg compensator.InterruptConfig
	err := config.UnmarshalKey("compensator.interrupt", &cfg)
	if err != nil {
		panic(err)
	}
	return compensator.NewInterruptCompensator(
		grpcClients,
		execSvc,
		cfg,
	)
}

func InitTerminationCompensator(service termination.Service) *compensator.TerminationCompensator {
	var cfg compensator.TerminationConfig
	if err := config.UnmarshalKey("compensator.termination", &cfg); err != nil {
		panic(err)
	}
	return compensator.NewTerminationCompensator(service, cfg)
}
