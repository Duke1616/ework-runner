//go:build wireinject
// +build wireinject

package ioc

import (
	"github.com/Duke1616/etask/internal/agent"
	"github.com/Duke1616/etask/sdk/executor/node"
	"github.com/google/wire"
)

// InitBase 初始化所有运行模式共享的服务发现基础设施。
func InitBase() *Base {
	wire.Build(BaseSet, wire.Struct(new(Base), "*"))
	return nil
}

// InitExecutionRuntime 初始化 Agent 和 Executor 共享的本地执行能力。
func InitExecutionRuntime() *ExecutionRuntime {
	wire.Build(ExecutionRuntimeSet, wire.Struct(new(ExecutionRuntime), "*"))
	return nil
}

// InitSchedulerApplication 使用一套共享依赖构建调度器、Web、gRPC 和后台任务。
func InitSchedulerApplication(base *Base) *SchedulerApplication {
	wire.Build(
		TaskSet,
		CodebookSet,
		CodeAssistSet,
		ArtifactSet,
		RunnerSet,
		VariableSet,
		PreviewSet,
		TaskExecutionSet,
		BindingResolverSet,
		ExecutorSet,
		ExecutionPoolSet,
		SchedulerSet,
		CompensatorSet,
		ConsumerSet,
		ProducerSet,
		GrpcSet,
		AppSet,
		WebSetup,
		EventSet,
		InitMQ,
		InitRoutePlanner,
		InitDispatcher,
		InitInvoker,
		wire.FieldsOf(new(*Base), "Registry", "Etcd"),
		wire.Struct(new(SchedulerApplication), "*"),
	)
	return nil
}

// InitExecutorModule 构造原生 Executor 模块。
func InitExecutorModule(base *Base, runtime *ExecutionRuntime) *node.Executor {
	wire.Build(
		InitExecutor,
		wire.FieldsOf(new(*Base), "Etcd"),
		wire.FieldsOf(new(*ExecutionRuntime), "ArtifactPreparer", "ScriptRuntime"),
	)
	return nil
}

// InitAgentModule 构造 Kafka Agent 模块。
func InitAgentModule(base *Base, runtime *ExecutionRuntime) *agent.Module {
	wire.Build(
		AgentSet,
		InitMQ,
		wire.FieldsOf(new(*Base), "Etcd"),
		wire.FieldsOf(new(*ExecutionRuntime), "ArtifactPreparer", "ScriptRuntime"),
	)
	return nil
}
