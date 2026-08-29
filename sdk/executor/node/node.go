package node

import (
	"context"

	"github.com/Duke1616/etask/pkg/grpc/registry"
	"github.com/Duke1616/etask/sdk/executor"
	"github.com/Duke1616/etask/sdk/executor/artifact"
	"github.com/Duke1616/etask/sdk/executor/internal/runtime"
	"github.com/gotomicro/ego/server"
)

// Executor 是标准 gRPC 任务执行节点。
type Executor struct {
	inner *runtime.Executor
}

// Option 配置节点可选的基础设施能力。
type Option func(*options)

type options struct {
	artifacts artifact.Preparer
}

// New 创建、注册并初始化可直接交给 EGO 启动的节点。
func New(config Config, reg registry.Registry, handlers ...executor.TaskHandler) (*Executor, error) {
	node, err := NewExecutor(config, reg)
	if err != nil {
		return nil, err
	}
	// 注册任务处理器
	if err = node.RegisterHandlers(handlers...); err != nil {
		return nil, err
	}
	// 初始化网络连接与服务组件
	if err = node.InitComponents(); err != nil {
		return nil, err
	}
	return node, nil
}

// WithArtifactPreparer 注入可选的本地制品物化能力。
func WithArtifactPreparer(preparer artifact.Preparer) Option {
	return func(options *options) { options.artifacts = preparer }
}

// NewExecutor 创建未初始化的节点，供高级装配和测试使用。
func NewExecutor(config Config, reg registry.Registry, opts ...Option) (*Executor, error) {
	applied := options{}
	for _, option := range opts {
		if option != nil {
			option(&applied)
		}
	}
	runtimeOptions := make([]runtime.Option, 0, 1)
	if applied.artifacts != nil {
		runtimeOptions = append(runtimeOptions, runtime.WithArtifactPreparer(applied.artifacts))
	}
	inner, err := runtime.NewExecutor(config.runtimeConfig(), reg, runtimeOptions...)
	if err != nil {
		return nil, err
	}
	return &Executor{inner: inner}, nil
}

// RegisterHandlers 校验并注册 Handler。
func (e *Executor) RegisterHandlers(handlers ...executor.TaskHandler) error {
	return e.inner.RegisterHandlers(handlers...)
}

// InitComponents 初始化调度客户端和 gRPC 服务端。
func (e *Executor) InitComponents() error { return e.inner.InitComponents() }

func (e *Executor) Name() string                           { return e.inner.Name() }
func (e *Executor) PackageName() string                    { return e.inner.PackageName() }
func (e *Executor) Init() error                            { return e.inner.Init() }
func (e *Executor) Start() error                           { return e.inner.Start() }
func (e *Executor) Stop() error                            { return e.inner.Stop() }
func (e *Executor) GracefulStop(ctx context.Context) error { return e.inner.GracefulStop(ctx) }
func (e *Executor) Info() *server.ServiceInfo              { return e.inner.Info() }

var _ server.Server = (*Executor)(nil)
