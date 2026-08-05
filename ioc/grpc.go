package ioc

import (
	"fmt"
	"os"
	"time"

	artifactv1 "github.com/Duke1616/etask/api/proto/gen/etask/artifact/v1"
	codebookv1 "github.com/Duke1616/etask/api/proto/gen/etask/codebook/v1"
	executorv1 "github.com/Duke1616/etask/api/proto/gen/etask/executor/v1"
	reporterv1 "github.com/Duke1616/etask/api/proto/gen/etask/reporter/v1"
	runnerv1 "github.com/Duke1616/etask/api/proto/gen/etask/runner/v1"
	schedulerv1 "github.com/Duke1616/etask/api/proto/gen/etask/scheduler/v1"
	taskv1 "github.com/Duke1616/etask/api/proto/gen/etask/task/v1"
	grpcapi "github.com/Duke1616/etask/internal/grpc"
	"github.com/Duke1616/etask/internal/grpc/scripts"
	"github.com/Duke1616/etask/pkg/config"
	grpcpkg "github.com/Duke1616/etask/pkg/grpc"
	"github.com/Duke1616/etask/pkg/grpc/pool"
	registrysdk "github.com/Duke1616/etask/pkg/grpc/registry"
	"github.com/Duke1616/etask/sdk/executor/artifact"
	"github.com/Duke1616/etask/sdk/executor/node"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
)

// InitExecutor 初始化原生 gRPC 执行器节点
func InitExecutor(reg registrysdk.Registry,
	artifactPreparer artifact.Preparer, scriptRuntime *scripts.Runtime) *node.Executor {
	var serverCfg grpcpkg.ServerConfig
	if err := config.UnmarshalKey("grpc.server.executor", &serverCfg); err != nil {
		panic(err)
	}

	var clientCfg grpcpkg.ClientConfig
	if err := config.UnmarshalKey("grpc.client.scheduler", &clientCfg); err != nil {
		panic(err)
	}
	if err := scriptRuntime.Initialize(); err != nil {
		panic(err)
	}

	cfg := node.Config{
		Mode:           resolveMode(),
		Desc:           viper.GetString("executor.desc"),
		IsolationLevel: viper.GetString("executor.isolation_level"),
		Server:         resolveServer(serverCfg),
		Client:         clientCfg,
	}

	exec, err := node.NewExecutor(cfg, reg,
		node.WithArtifactPreparer(artifactPreparer),
	)
	if err != nil {
		panic(err)
	}

	if err = exec.RegisterHandlers(scriptRuntime.Handlers()...); err != nil {
		panic(err)
	}

	// 立即初始化组件，确保 Server() 等方法能够返回有效对象
	if err = exec.InitComponents(); err != nil {
		panic(err)
	}

	return exec
}

// InitSchedulerNodeGRPCServer 初始化 Scheduler gRPC 服务器
func InitSchedulerNodeGRPCServer(registry registrysdk.Registry, reporter *grpcapi.ReporterServer,
	task *grpcapi.TaskServer, agent *grpcapi.AgentServer, codebook *grpcapi.CodebookServer,
	runner *grpcapi.RunnerServer, artifact *grpcapi.ArtifactServer,
	scheduler *grpcapi.SchedulerServer) *grpcpkg.Server {
	var cfg grpcpkg.ServerConfig
	if err := config.UnmarshalKey("grpc.server.scheduler", &cfg); err != nil {
		panic(err)
	}

	server := grpcpkg.NewServer(cfg, registry, grpcpkg.WithJWTAuth(cfg.AuthToken))
	reporterv1.RegisterReporterServiceServer(server.Server, reporter)
	taskv1.RegisterTaskServiceServer(server.Server, task)
	executorv1.RegisterAgentServiceServer(server.Server, agent)
	executorv1.RegisterTaskExecutionServiceServer(server.Server, agent)
	codebookv1.RegisterCodebookServiceServer(server.Server, codebook)
	runnerv1.RegisterRunnerServiceServer(server.Server, runner)
	artifactv1.RegisterArtifactServiceServer(server.Server, artifact)
	schedulerv1.RegisterSchedulerServiceServer(server.Server, scheduler)

	return server
}

func InitExecutorServiceGRPCClients(reg registrysdk.Registry) *pool.Clients[executorv1.ExecutorServiceClient] {
	const defaultTimeout = time.Second
	var cfg grpcpkg.ClientConfig
	if err := config.UnmarshalKey("grpc.client.executor", &cfg); err != nil {
		panic(err)
	}

	return pool.NewClients(
		reg,
		defaultTimeout,
		cfg.AuthToken,
		func(conn *grpc.ClientConn) executorv1.ExecutorServiceClient {
			return executorv1.NewExecutorServiceClient(conn)
		})
}

// resolveServer 确定最终的 NodeID
// 优先级：EXECUTOR_NODE_ID > executor.id > grpc.server.executor.id。
// 最终格式：serviceName:nodeID
func resolveServer(sc grpcpkg.ServerConfig) grpcpkg.ServerConfig {
	nodeID := os.Getenv("EXECUTOR_NODE_ID")

	if nodeID == "" {
		nodeID = viper.GetString("executor.id")
	}

	if nodeID == "" {
		nodeID = sc.ServiceId
	}

	if nodeID != "" {
		sc.ServiceId = fmt.Sprintf("%s:%s", sc.ServiceName, nodeID)
	}

	return sc
}

func resolveMode() string {
	mode := viper.GetString("executor.mode")
	if mode == "" {
		mode = "PUSH"
	}

	return mode
}
