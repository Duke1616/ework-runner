// Package engine 实现与传输协议无关的进程内任务执行编排。
package engine

import (
	"context"
	"errors"
	"fmt"

	"github.com/Duke1616/etask/sdk/executor/artifact"
	"github.com/Duke1616/etask/sdk/executor/internal/task"
)

// Command 描述执行一次任务所需的全部进程内输入。
type Command struct {
	Task            task.TaskInfo
	Params          map[string]string
	Metadata        map[string]string
	Parameters      []task.Parameter
	Artifacts       []artifact.Ref
	ExecutionLogger task.ExecutionLogger
}

// Result 描述一次 Handler 执行产生的结构化结果。
type Result struct {
	Value string
}

// Engine 统一编排制品准备、Context 创建和 Handler 调用。
type Engine struct {
	handlers     *task.HandlerRegistry
	artifacts    artifact.Preparer
	downloader   artifact.Downloader
	progress     task.ProgressReporter
	systemLogger task.SystemLogger
}

// Option 配置 Engine 生命周期内稳定持有的基础设施依赖。
type Option func(*Engine)

// WithArtifactDownloader 注入与传输协议无关的制品下载端口。
func WithArtifactDownloader(downloader artifact.Downloader) Option {
	return func(engine *Engine) { engine.downloader = downloader }
}

// WithProgressReporter 注入运行环境的进度状态端口。
func WithProgressReporter(reporter task.ProgressReporter) Option {
	return func(engine *Engine) { engine.progress = reporter }
}

// WithSystemLogger 注入 Engine 和任务 Context 使用的系统日志端口。
func WithSystemLogger(logger task.SystemLogger) Option {
	return func(engine *Engine) { engine.systemLogger = logger }
}

// New 创建进程内执行引擎。
func New(handlers *task.HandlerRegistry, artifacts artifact.Preparer, options ...Option) *Engine {
	engine := &Engine{handlers: handlers, artifacts: artifacts}
	for _, option := range options {
		if option != nil {
			option(engine)
		}
	}
	return engine
}

// Execute 准备任务运行现场并同步执行 Handler。
func (e *Engine) Execute(ctx context.Context, command Command) (result Result, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	executionLogger := command.ExecutionLogger
	// 执行引擎持有执行日志器，并保证所有返回路径都会关闭它。
	taskCtx := task.NewContext(task.ContextOptions{
		Context: ctx, Task: command.Task, Params: command.Params,
		Metadata: command.Metadata, Parameters: command.Parameters,
		SystemLogger: scopedSystemLogger(e.systemLogger, command.Task), ExecutionLogger: executionLogger,
		Progress: e.progress,
	})
	defer taskCtx.Close()
	if e.handlers == nil {
		return Result{}, fmt.Errorf("任务处理器注册中心尚未初始化")
	}
	handler, exists := e.handlers.Get(command.Task.Handler)
	if !exists {
		return Result{}, fmt.Errorf("未找到任务处理器: %s", command.Task.Handler)
	}
	// 执行引擎是扩展 Handler 的最后一道隔离边界，panic 时仍保留已产生的结果。
	defer func() {
		if recovered := recover(); recovered != nil {
			result.Value, err = taskCtx.Result()
			err = errors.Join(fmt.Errorf("任务处理器发生 panic: %v", recovered), err)
		}
	}()

	// 准备器返回本次任务固定的制品根；Close 释放实现可能持有的任务级资源。
	prepared, err := e.prepareArtifacts(ctx, command)
	if err != nil {
		return Result{}, err
	}
	if prepared != nil {
		defer func() {
			if closeErr := prepared.Close(); closeErr != nil {
				err = errors.Join(err, fmt.Errorf("清理制品运行现场失败: %w", closeErr))
			}
		}()
		taskCtx.SetArtifactRoots(prepared.Roots())
	}
	// Handler 只接触稳定的 Context，不感知下载、缓存和传输协议。
	if err = handler.Run(taskCtx); err != nil {
		value, resultErr := taskCtx.Result()
		return Result{Value: value}, errors.Join(err, resultErr)
	}
	value, err := taskCtx.Result()
	return Result{Value: value}, err
}

// Prune 清理制品准备器维护的本地缓存。
func (e *Engine) Prune() error {
	if e.artifacts == nil {
		return nil
	}
	return e.artifacts.Prune()
}

func (e *Engine) prepareArtifacts(ctx context.Context, command Command) (artifact.PreparedArtifacts, error) {
	if len(command.Artifacts) == 0 {
		return nil, nil
	}
	if e.artifacts == nil {
		return nil, fmt.Errorf("任务声明了制品，但执行引擎未配置制品准备器")
	}
	prepared, err := e.artifacts.Prepare(ctx, e.downloader, command.Artifacts)
	if err != nil {
		return nil, fmt.Errorf("准备代码制品失败: %w", err)
	}
	return prepared, nil
}
