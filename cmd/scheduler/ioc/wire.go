//go:build wireinject

package ioc

import (
	"github.com/Duke1616/ework-runner/internal/grpc"
	"github.com/Duke1616/ework-runner/internal/repository"
	"github.com/Duke1616/ework-runner/internal/repository/dao"
	taskSvc "github.com/Duke1616/ework-runner/internal/service/task"
	"github.com/Duke1616/ework-runner/internal/web/task"
	"github.com/Duke1616/ework-runner/ioc"
	"github.com/Duke1616/ework-runner/pkg/ginx/middleware"
	"github.com/google/wire"
)

var (
	BaseSet = wire.NewSet(
		ioc.InitDB,
		ioc.InitRedis,
		ioc.InitDistributedLock,
		ioc.InitEtcdClient,
		ioc.InitMQ,
		ioc.InitRunner,
		ioc.InitInvoker,
		ioc.InitRegistry,
	)

	webSetup = wire.NewSet(
		ioc.InitECMDBGrpcClient,
		ioc.InitPolicyServiceClient,
		middleware.NewCheckPolicyMiddlewareBuilder,
		ioc.InitSession,
		ioc.InitGinMiddlewares,
		ioc.InitGinWebServer,
	)

	taskSet = wire.NewSet(
		dao.NewGORMTaskDAO,
		repository.NewTaskRepository,
		taskSvc.NewService,
		task.NewHandler,
	)

	taskExecutionSet = wire.NewSet(
		dao.NewGORMTaskExecutionDAO,
		repository.NewTaskExecutionRepository,
		taskSvc.NewExecutionService,
	)

	schedulerSet = wire.NewSet(
		ioc.InitNodeID,
		ioc.InitScheduler,
		ioc.InitMySQLTaskAcquirer,
		ioc.InitExecutorNodePicker,
	)

	compensatorSet = wire.NewSet(
		ioc.InitRetryCompensator,
		ioc.InitRescheduleCompensator,
		ioc.InitInterruptCompensator,
	)

	producerSet = wire.NewSet(
		ioc.InitCompleteProducer,
	)

	grpcSet = wire.NewSet(
		ioc.InitExecutorServiceGRPCClients,
	)

	consumerSet = wire.NewSet(
		ioc.InitCompleteEventConsumer,
	)
)

func InitSchedulerApp() *ioc.SchedulerApp {
	wire.Build(
		// 基础设施
		BaseSet,

		taskSet,
		taskExecutionSet,
		schedulerSet,
		compensatorSet,
		consumerSet,
		producerSet,
		grpcSet,

		// WEB 服务
		webSetup,

		// GRPC服务器
		grpc.NewReporterServer,
		ioc.InitSchedulerNodeGRPCServer,
		ioc.InitTasks,
		wire.Struct(new(ioc.SchedulerApp), "*"),
	)

	return new(ioc.SchedulerApp)
}
