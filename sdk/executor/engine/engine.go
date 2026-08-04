// Package engine 提供 etask Agent 等自定义传输使用的进程内执行管线。
// 大多数标准 Executor 实现只需要使用上级 executor 包。
package engine

import (
	"context"

	artifactv1 "github.com/Duke1616/etask/api/proto/gen/etask/artifact/v1"
	reporterv1 "github.com/Duke1616/etask/api/proto/gen/etask/reporter/v1"
	"github.com/Duke1616/etask/sdk/executor"
	"github.com/Duke1616/etask/sdk/executor/artifact"
	internalengine "github.com/Duke1616/etask/sdk/executor/internal/engine"
	"github.com/Duke1616/etask/sdk/executor/internal/task"
	"github.com/gotomicro/ego/core/elog"
)

// Command 描述传输适配器提交的一次任务。
type Command struct {
	Context    context.Context
	Task       executor.TaskInfo
	Params     map[string]string
	Metadata   map[string]string
	Parameters []executor.Parameter
	Artifacts  []*artifactv1.ArtifactRef
	TaskLogger executor.TaskLogger
}

// Result 表示一次执行返回的结构化结果。
type Result struct {
	Value string
}

// Engine 统一编排制品准备和 Handler 调用。
type Engine struct {
	inner *internalengine.Engine
}

// Option 配置 Engine 生命周期内持有的基础设施。
type Option func(*options)

type options struct {
	artifactClient artifactv1.ArtifactServiceClient
	reporter       reporterv1.ReporterServiceClient
	progress       executor.ProgressReporter
	logger         *elog.Component
}

// New 创建供自定义传输使用的执行引擎。
func New(handlers *HandlerRegistry, artifacts artifact.Preparer, opts ...Option) *Engine {
	applied := options{}
	for _, option := range opts {
		if option != nil {
			option(&applied)
		}
	}
	return &Engine{inner: internalengine.New(internalHandlerRegistry(handlers), artifacts,
		internalengine.WithArtifactClient(applied.artifactClient),
		internalengine.WithReporter(applied.reporter),
		internalengine.WithProgressReporter(applied.progress),
		internalengine.WithLogger(applied.logger),
	)}
}

// WithArtifactClient 将制品下载客户端绑定到 Engine 生命周期。
func WithArtifactClient(client artifactv1.ArtifactServiceClient) Option {
	return func(options *options) { options.artifactClient = client }
}

// WithReporter 将日志和状态上报客户端绑定到 Engine 生命周期。
func WithReporter(reporter reporterv1.ReporterServiceClient) Option {
	return func(options *options) { options.reporter = reporter }
}

// WithProgressReporter 将进度上报端口绑定到 Engine 生命周期。
func WithProgressReporter(reporter executor.ProgressReporter) Option {
	return func(options *options) { options.progress = reporter }
}

// WithLogger 将系统日志器绑定到 Engine 生命周期。
func WithLogger(logger *elog.Component) Option {
	return func(options *options) { options.logger = logger }
}

// Execute 同步执行一次任务。
func (e *Engine) Execute(ctx context.Context, command Command) (Result, error) {
	result, err := e.inner.Execute(ctx, internalengine.Command{
		Context: command.Context, Task: command.Task, Params: command.Params,
		Metadata: command.Metadata, Parameters: command.Parameters, Artifacts: command.Artifacts,
		TaskLogger: command.TaskLogger,
	})
	return Result{Value: result.Value}, err
}

// Prune 清理制品准备器维护的缓存。
func (e *Engine) Prune() error { return e.inner.Prune() }

// HandlerRegistry 保存 Engine 使用的 Handler。
type HandlerRegistry struct {
	inner *task.HandlerRegistry
}

// NewHandlerRegistry 创建空的 Handler 注册表。
func NewHandlerRegistry() *HandlerRegistry {
	return &HandlerRegistry{inner: task.NewHandlerRegistry()}
}

// Register 保留不返回错误的注册行为。
// 需要立即获取校验错误时应使用 RegisterChecked。
func (r *HandlerRegistry) Register(handlers ...executor.TaskHandler) {
	_ = r.RegisterChecked(handlers...)
}

// RegisterChecked 校验并注册 Handler。
func (r *HandlerRegistry) RegisterChecked(handlers ...executor.TaskHandler) error {
	return r.inner.RegisterChecked(handlers...)
}

// Get 按名称返回 Handler。
func (r *HandlerRegistry) Get(name string) (executor.TaskHandler, bool) { return r.inner.Get(name) }

// ListMetas 返回按名称排序的 Handler 元数据。
func (r *HandlerRegistry) ListMetas() []executor.HandlerMeta { return r.inner.ListMetas() }

// Names 返回按名称排序的 Handler 名称。
func (r *HandlerRegistry) Names() []string { return r.inner.Names() }

// Snapshot 返回 Handler 映射的副本。
func (r *HandlerRegistry) Snapshot() map[string]executor.TaskHandler { return r.inner.Snapshot() }

func internalHandlerRegistry(registry *HandlerRegistry) *task.HandlerRegistry {
	if registry == nil {
		return nil
	}
	return registry.inner
}
