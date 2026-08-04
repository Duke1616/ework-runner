package runtime

import (
	"context"
	"errors"
	"fmt"
	"sync"

	artifactv1 "github.com/Duke1616/etask/api/proto/gen/etask/artifact/v1"
	executorv1 "github.com/Duke1616/etask/api/proto/gen/etask/executor/v1"
	reporterv1 "github.com/Duke1616/etask/api/proto/gen/etask/reporter/v1"
	grpcpkg "github.com/Duke1616/etask/pkg/grpc"
	"github.com/Duke1616/etask/pkg/grpc/registry"
	"github.com/Duke1616/etask/sdk/executor/artifact"
	enginepkg "github.com/Duke1616/etask/sdk/executor/internal/engine"
	"github.com/Duke1616/etask/sdk/executor/internal/execution"
	"github.com/Duke1616/etask/sdk/executor/internal/task"
	"github.com/gotomicro/ego/core/elog"
	"google.golang.org/grpc"
)

// Executor 实现执行节点服务。
type Executor struct {
	executorv1.UnimplementedExecutorServiceServer

	config   Config
	registry registry.Registry
	hr       *task.HandlerRegistry

	// 内部组件
	server          *grpcpkg.Server
	schedulerConn   *grpc.ClientConn
	reporterClient  reporterv1.ReporterServiceClient
	agentClient     executorv1.AgentServiceClient
	executionClient executorv1.TaskExecutionServiceClient
	artifactClient  artifactv1.ArtifactServiceClient
	artifacts       artifact.Preparer
	engine          *enginepkg.Engine
	logger          *elog.Component

	executions      executionStore
	pullCancel      context.CancelFunc
	initMu          sync.Mutex
	initialized     bool
	registrationErr error
	runMu           sync.Mutex
	runWG           sync.WaitGroup
	stopping        bool
}

// NewExecutor 创建 Executor
// Option 配置 Executor 的可选基础设施能力。
type Option func(*Executor)

// WithArtifactPreparer 注入可选的制品本地物化实现。
func WithArtifactPreparer(preparer artifact.Preparer) Option {
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
	_ = e.RegisterHandlers(handlers...)
	return e
}

// RegisterHandlers 注册处理器并返回输入冲突；组件初始化后不再允许修改能力集。
func (e *Executor) RegisterHandlers(handlers ...task.TaskHandler) error {
	e.initMu.Lock()
	defer e.initMu.Unlock()
	if e.initialized {
		return fmt.Errorf("Executor 已初始化，不能再注册任务处理器")
	}
	err := e.hr.RegisterChecked(handlers...)
	if err != nil {
		e.registrationErr = errors.Join(e.registrationErr, err)
	}
	return err
}
