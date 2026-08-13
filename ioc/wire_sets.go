package ioc

import (
	agentSvc "github.com/Duke1616/etask/internal/agent"
	"github.com/Duke1616/etask/internal/grpc"
	"github.com/Duke1616/etask/internal/repository"
	"github.com/Duke1616/etask/internal/repository/dao"
	artifactSvc "github.com/Duke1616/etask/internal/service/artifact"
	codeassistSvc "github.com/Duke1616/etask/internal/service/codeassist"
	codebookSvc "github.com/Duke1616/etask/internal/service/codebook"
	poolSvc "github.com/Duke1616/etask/internal/service/pool"
	previewSvc "github.com/Duke1616/etask/internal/service/preview"
	programSvc "github.com/Duke1616/etask/internal/service/program"
	runnerSvc "github.com/Duke1616/etask/internal/service/runner"
	submissionSvc "github.com/Duke1616/etask/internal/service/submission"
	taskSvc "github.com/Duke1616/etask/internal/service/task"
	taskBinding "github.com/Duke1616/etask/internal/service/task/binding"
	terminationSvc "github.com/Duke1616/etask/internal/service/termination"
	variableSvc "github.com/Duke1616/etask/internal/service/variable"
	internalSSE "github.com/Duke1616/etask/internal/sse"
	artifactWeb "github.com/Duke1616/etask/internal/web/artifact"
	codeassistWeb "github.com/Duke1616/etask/internal/web/codeassist"
	codebookWeb "github.com/Duke1616/etask/internal/web/codebook"
	"github.com/Duke1616/etask/internal/web/manager"
	poolWeb "github.com/Duke1616/etask/internal/web/pool"
	previewWeb "github.com/Duke1616/etask/internal/web/preview"
	resourceWeb "github.com/Duke1616/etask/internal/web/resource"
	runnerWeb "github.com/Duke1616/etask/internal/web/runner"
	variableWeb "github.com/Duke1616/etask/internal/web/variable"
	"github.com/google/wire"
)

var (
	BaseSet = wire.NewSet(
		InitEtcdClient,
		InitRegistry,
	)

	ExecutionRuntimeSet = wire.NewSet(
		InitArtifactPreparer,
		InitScriptRuntime,
	)

	EventSet = wire.NewSet(
		InitRedis,
		internalSSE.NewHubs,
	)

	WebSetup = wire.NewSet(
		InitPolicySDK,
		InitPermSyncer,
		InitProviders,
		InitListener,
		InitGinMiddlewares,
		InitGinWebServer,
	)

	TaskSet = wire.NewSet(
		InitDB,
		dao.NewGORMTaskDAO,
		repository.NewTaskRepository,
		repository.NewTaskExecutionLogRepository,
		taskSvc.NewService,
		taskSvc.NewLogService,
		manager.NewHandler,
	)

	CodebookSet = wire.NewSet(
		dao.NewGORMCodebookDAO,
		dao.NewGORMCodebookProjectDAO,
		dao.NewGORMProjectDeletionDAO,
		repository.NewCodebookRepository,
		repository.NewProjectDeletionRepository,
		codebookSvc.NewService,
		codebookSvc.NewProjectDeletionService,
		wire.Bind(new(codebookSvc.ProjectFileRepository), new(repository.ICodebookRepository)),
		codebookSvc.NewProjectFileService,
		wire.Bind(new(programSvc.CodebookReader), new(codebookSvc.Service)),
		codebookSvc.NewWorkspaceService,
		wire.Bind(new(codebookSvc.WorkspaceSourceReader), new(repository.ICodebookRepository)),
		codebookWeb.NewHandler,
	)

	CodeAssistSet = wire.NewSet(
		InitAIProvider,
		dao.NewGORMCodeAssistDAO,
		repository.NewCodeAssistRepository,
		codeassistSvc.NewService,
		codeassistWeb.NewHandler,
	)

	ArtifactSet = wire.NewSet(
		dao.NewGORMArtifactDAO,
		dao.NewGORMProjectSourceDAO,
		repository.NewArtifactRepository,
		repository.NewProjectSourceRepository,
		InitArtifactConfig,
		InitArtifactArchive,
		InitArtifactStore,
		artifactSvc.NewService,
		programSvc.NewService,
		wire.Bind(new(codebookSvc.WorkspaceArtifactReader), new(artifactSvc.Service)),
		artifactWeb.NewHandler,
	)

	RunnerSet = wire.NewSet(
		dao.NewGORMRunnerDAO,
		dao.NewGORMVariableDAO,
		InitCrypto,
		repository.NewRunnerRepository,
		runnerSvc.NewService,
		runnerWeb.NewHandler,
	)

	VariableSet = wire.NewSet(
		repository.NewVariableRepository,
		variableSvc.NewService,
		variableWeb.NewHandler,
	)

	PreviewSet = wire.NewSet(
		previewSvc.NewService,
		previewWeb.NewHandler,
	)

	BindingResolverSet = wire.NewSet(
		taskBinding.NewScriptBindingResolvers,
	)

	ExecutionPoolCoreSet = wire.NewSet(
		dao.NewGORMExecutionPoolDAO,
		dao.NewGORMExecutionPoolBindingDAO,
		repository.NewExecutionPoolRepository,
		repository.NewExecutionPoolBindingRepository,
	)

	ExecutionPoolBindingSet = wire.NewSet(
		ExecutionPoolCoreSet,
		poolSvc.NewBindingService,
		poolSvc.NewCatalogService,
		poolWeb.NewAdminHandler,
	)

	ExecutionPoolSet = wire.NewSet(
		ExecutionPoolBindingSet,
		poolSvc.NewSyncer,
	)

	ExecutorSet = wire.NewSet(
		resourceWeb.NewHandler,
	)

	TaskExecutionSet = wire.NewSet(
		dao.NewGORMTaskExecutionDAO,
		dao.NewGORMTaskExecutionLogDAO,
		dao.NewGORMExecutionCancellationDAO,
		repository.NewTaskExecutionRepository,
		repository.NewExecutionCancellationRepository,
		taskSvc.NewExecutionService,
		terminationSvc.NewService,
		wire.Bind(new(grpc.ExecutionReportHandler), new(taskSvc.ExecutionService)),
	)

	SchedulerSet = wire.NewSet(
		InitNodeID,
		InitScheduler,
		InitMySQLTaskAcquirer,
		InitExecutorNodePicker,
	)

	AgentSet = wire.NewSet(
		agentSvc.InitModule,
	)

	CompensatorSet = wire.NewSet(
		InitRetryCompensator,
		InitRescheduleCompensator,
		InitInterruptCompensator,
		InitTerminationCompensator,
	)

	ProducerSet = wire.NewSet(
		InitCompleteProducer,
	)

	GrpcSet = wire.NewSet(
		InitExecutorServiceGRPCClients,
	)

	ConsumerSet = wire.NewSet(
		InitCompleteEventConsumer,
		InitAgentEventConsumer,
	)

	// AppSet 包含 Scheduler 模式的核心 Provider
	AppSet = wire.NewSet(
		grpc.NewReporterServer,
		grpc.NewTaskServer,
		grpc.NewAgentServer,
		grpc.NewCodebookServer,
		grpc.NewRunnerServer,
		grpc.NewArtifactServer,
		submissionSvc.NewService,
		grpc.NewSchedulerServer,
		InitTasks,
		InitSchedulerNodeGRPCServer,
	)
)
