package runtime

import (
	"context"
	"sync"

	artifactv1 "github.com/Duke1616/etask/api/proto/gen/etask/artifact/v1"
	executorv1 "github.com/Duke1616/etask/api/proto/gen/etask/executor/v1"
	reporterv1 "github.com/Duke1616/etask/api/proto/gen/etask/reporter/v1"
	grpcpkg "github.com/Duke1616/etask/pkg/grpc"
	"github.com/Duke1616/etask/pkg/grpc/registry"
	"github.com/Duke1616/etask/sdk/executor/internal/artifactport"
	enginepkg "github.com/Duke1616/etask/sdk/executor/internal/engine"
	"github.com/Duke1616/etask/sdk/executor/internal/execution"
	"github.com/Duke1616/etask/sdk/executor/internal/task"
	"github.com/gotomicro/ego/core/elog"
)

// Executor 实现执行节点服务。
type Executor struct {
	executorv1.UnimplementedExecutorServiceServer

	config   Config
	registry registry.Registry
	hr       *task.HandlerRegistry

	// 内部组件
	server          *grpcpkg.Server
	reporterClient  reporterv1.ReporterServiceClient
	agentClient     executorv1.AgentServiceClient
	executionClient executorv1.TaskExecutionServiceClient
	artifactClient  artifactv1.ArtifactServiceClient
	artifacts       artifactport.Preparer
	engine          *enginepkg.Engine
	logger          *elog.Component

	executions  executionStore
	pullCancel  context.CancelFunc
	initMu      sync.Mutex
	initialized bool
}

// NewExecutor 创建 Executor
// Option 配置 Executor 的可选基础设施能力。
type Option func(*Executor)

// WithArtifactPreparer 注入可选的制品本地物化实现。
func WithArtifactPreparer(preparer artifactport.Preparer) Option {
	return func(executor *Executor) {
		executor.artifacts = preparer
	}
}

func NewExecutor(cfg Config, reg registry.Registry, options ...Option) (*Executor, error) {
	// 先统一默认配置和运行模式，后续组件只接收可直接使用的配置。
	config, err := normalizeConfig(cfg, reg)
	if err != nil {
		return nil, err
	}

	executor := &Executor{
		config:     config,
		registry:   reg,
		hr:         task.NewHandlerRegistry(),
		logger:     elog.DefaultLogger.With(elog.FieldComponentName("executor")),
		executions: execution.NewStore(),
	}
	for _, option := range options {
		if option != nil {
			option(executor)
		}
	}
	// Engine 最后装配，以便可选的制品准备器能够参与每次任务执行。
	executor.engine = enginepkg.New(executor.hr, executor.artifacts)
	return executor, nil
}

// RegisterHandler 注册任务处理函数
func (e *Executor) RegisterHandler(handlers ...task.TaskHandler) *Executor {
	e.hr.Register(handlers...)
	return e
}
