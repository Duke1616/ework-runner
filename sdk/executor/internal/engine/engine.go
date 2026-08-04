// Package engine 实现与传输协议无关的进程内任务执行编排。
package engine

import (
	"context"
	"errors"
	"fmt"

	artifactv1 "github.com/Duke1616/etask/api/proto/gen/etask/artifact/v1"
	reporterv1 "github.com/Duke1616/etask/api/proto/gen/etask/reporter/v1"
	"github.com/Duke1616/etask/sdk/executor/internal/artifactport"
	"github.com/Duke1616/etask/sdk/executor/internal/task"
	"github.com/gotomicro/ego/core/elog"
)

// Command 描述执行一次任务所需的全部进程内输入。
type Command struct {
	Context        context.Context
	Task           task.TaskInfo
	Params         map[string]string
	Metadata       map[string]string
	Parameters     []task.Parameter
	Artifacts      []*artifactv1.ArtifactRef
	ArtifactClient artifactv1.ArtifactServiceClient
	Logger         *elog.Component
	TaskLogger     task.Logger
	Reporter       reporterv1.ReporterServiceClient
}

// Result 描述一次 Handler 执行产生的结构化结果。
type Result struct {
	Value string
}

// Engine 统一编排制品准备、Context 创建和 Handler 调用。
type Engine struct {
	handlers  *task.HandlerRegistry
	artifacts artifactport.Preparer
}

// New 创建进程内执行引擎。
func New(handlers *task.HandlerRegistry, artifacts artifactport.Preparer) *Engine {
	return &Engine{handlers: handlers, artifacts: artifacts}
}

// Execute 准备任务运行现场并同步执行 Handler。
func (e *Engine) Execute(ctx context.Context, command Command) (result Result, err error) {
	// 将调用方上下文规范化一次，避免 Handler 和制品准备阶段分别处理 nil。
	if command.Context == nil {
		command.Context = ctx
	}
	if command.Context == nil {
		command.Context = context.Background()
	}
	// 执行引擎持有任务日志器，并保证所有返回路径都会关闭它。
	taskCtx := task.NewContext(task.ContextOptions{
		Context: command.Context, Task: command.Task, Params: command.Params,
		Metadata: command.Metadata, Parameters: command.Parameters,
		Logger: command.Logger, TaskLogger: command.TaskLogger, Reporter: command.Reporter,
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
			result.Value = taskCtx.ResultJSON()
			err = fmt.Errorf("任务处理器发生 panic: %v", recovered)
		}
	}()

	// 准备器返回本次任务固定的制品根；Close 兼容需要任务级现场的旧实现。
	prepared, err := e.prepareArtifacts(command)
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
		return Result{Value: taskCtx.ResultJSON()}, err
	}
	return Result{Value: taskCtx.ResultJSON()}, nil
}

// Prune 清理制品准备器维护的本地缓存。
func (e *Engine) Prune() error {
	if e.artifacts == nil {
		return nil
	}
	return e.artifacts.Prune()
}

func (e *Engine) prepareArtifacts(command Command) (artifactport.PreparedArtifacts, error) {
	if len(command.Artifacts) == 0 {
		return nil, nil
	}
	if e.artifacts == nil {
		return nil, fmt.Errorf("任务声明了制品，但执行引擎未配置制品准备器")
	}
	prepared, err := e.artifacts.Prepare(command.Context, command.ArtifactClient, command.Artifacts)
	if err != nil {
		return nil, fmt.Errorf("准备代码制品失败: %w", err)
	}
	return prepared, nil
}
