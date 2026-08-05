// Package engine 提供 etask Agent 等自定义传输使用的进程内执行管线。
// 大多数标准 Executor 实现只需要使用上级 executor 包。
package engine

import (
	"context"

	"github.com/Duke1616/etask/sdk/executor"
	"github.com/Duke1616/etask/sdk/executor/artifact"
	internalengine "github.com/Duke1616/etask/sdk/executor/internal/engine"
	"github.com/Duke1616/etask/sdk/executor/internal/task"
)

// Command 描述传输适配器提交的一次任务。
type Command struct {
	Task       executor.TaskInfo
	Params     map[string]string
	Metadata   map[string]string
	Parameters []executor.Parameter
	Artifacts  []artifact.Ref
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
	downloader artifact.Downloader
	progress   executor.ProgressReporter
	logger     executor.SystemLogger
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
		internalengine.WithArtifactDownloader(applied.downloader),
		internalengine.WithProgressReporter(applied.progress),
		internalengine.WithLogger(applied.logger),
	)}
}

// WithArtifactDownloader 将传输无关的制品下载端口绑定到 Engine 生命周期。
func WithArtifactDownloader(downloader artifact.Downloader) Option {
	return func(options *options) { options.downloader = downloader }
}

// WithProgressReporter 将进度上报端口绑定到 Engine 生命周期。
func WithProgressReporter(reporter executor.ProgressReporter) Option {
	return func(options *options) { options.progress = reporter }
}

// WithLogger 将系统日志端口绑定到 Engine 生命周期。
func WithLogger(logger executor.SystemLogger) Option {
	return func(options *options) { options.logger = logger }
}

// Execute 同步执行一次任务。
func (e *Engine) Execute(ctx context.Context, command Command) (Result, error) {
	result, err := e.inner.Execute(ctx, internalengine.Command{
		Task: command.Task, Params: command.Params,
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

// Register 校验并注册 Handler。
func (r *HandlerRegistry) Register(handlers ...executor.TaskHandler) error {
	return r.inner.Register(handlers...)
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
