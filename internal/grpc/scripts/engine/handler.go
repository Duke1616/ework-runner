package engine

import (
	"fmt"
	"slices"

	"github.com/Duke1616/etask/sdk/executor"
	"github.com/gotomicro/ego/core/elog"
)

// Handler 编排脚本参数解析、工作区、语言命令、输出采集和归档。
type Handler struct {
	config     Config
	adapter    Adapter
	workspaces WorkspaceFactory
	archiver   Archiver
	launcher   ProcessLauncher
}

// NewHandler 创建面向端口依赖的脚本处理器。
func NewHandler(config Config, adapter Adapter, workspaces WorkspaceFactory, archiver Archiver,
	launcher ProcessLauncher) (*Handler, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if adapter == nil || workspaces == nil || archiver == nil || launcher == nil {
		return nil, fmt.Errorf("脚本处理器依赖不能为空")
	}
	return &Handler{
		config: config, adapter: adapter, workspaces: workspaces, archiver: archiver, launcher: launcher,
	}, nil
}

// Name 返回语言 handler 名称。
func (h *Handler) Name() string {
	return h.adapter.Name()
}

// Desc 返回语言 handler 描述。
func (h *Handler) Desc() string {
	return h.adapter.Description()
}

// ProgramKinds 返回脚本 Handler 支持的程序来源类型。
func (h *Handler) ProgramKinds() []executor.ProgramKind {
	return append([]executor.ProgramKind(nil), h.adapter.ProgramKinds()...)
}

// Metadata 返回语言 handler 支持的参数元数据。
func (h *Handler) Metadata() []executor.Parameter {
	return h.adapter.Metadata()
}

// Run 执行脚本并将日志和结果写回 executor.Context。
func (h *Handler) Run(task *executor.Context) (runErr error) {
	// 绑定解析和大小校验在写入任何运行文件前完成。
	request, err := resolveRequest(task, h.adapter.Name(), h.adapter.Metadata(), h.config)
	if err != nil {
		return err
	}
	if !slices.Contains(h.adapter.ProgramKinds(), request.program.Kind) {
		return fmt.Errorf("[%s] 不支持 %s 程序来源", h.adapter.Name(), request.program.Kind)
	}
	workspace, err := h.workspaces.Create(WorkspaceOptions{
		ExecutionID: task.ExecutionID(),
		Extension:   h.adapter.Extension(),
		Program:     request.program,
		Artifacts:   task.ArtifactRoots(),
	})
	if err != nil {
		return fmt.Errorf("创建脚本工作区失败: %w", err)
	}
	// 先归档再删除工作区，确保失败现场仍能读取到脚本文件。
	defer func() {
		if archiveErr := h.archiver.Archive(ArchiveRecord{
			ExecutionID: task.ExecutionID(),
			CodeFile:    workspace.EntryPoint(),
			Args:        request.input.Args, Variables: request.input.Variables,
			Failed: runErr != nil,
		}); archiveErr != nil {
			task.SystemLogger().Error("归档脚本执行现场失败", elog.FieldErr(archiveErr))
		}
		if closeErr := workspace.Close(); closeErr != nil {
			task.SystemLogger().Error("清理脚本工作区失败", elog.FieldErr(closeErr))
		}
	}()

	// 语言适配器只负责生成命令，通用 Runner 统一处理环境、日志和结果流。
	prepared, err := h.adapter.Prepare(task.Context(), workspace, request.input)
	if err != nil {
		return fmt.Errorf("准备 %s 执行命令失败: %w", h.adapter.Name(), err)
	}
	task.AddSecretMasks(prepared.SecretMasks...)
	runner := commandRunner{
		maxLogLineSize: h.config.MaxLogLineSize,
		maxResultSize:  h.config.MaxResultSize,
		launcher:       h.launcher,
	}
	return runner.Run(task, workspace, prepared)
}

var _ executor.TaskHandler = (*Handler)(nil)
