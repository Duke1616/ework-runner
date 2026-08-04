package executor

import (
	"github.com/Duke1616/etask/sdk/executor/internal/artifactport"
	"github.com/Duke1616/etask/sdk/executor/internal/task"
)

type (
	// TaskInfo 描述一次任务执行的只读身份信息。
	TaskInfo = task.TaskInfo
	// ContextOptions 描述创建任务上下文所需的输入和依赖。
	ContextOptions = task.ContextOptions
	// Context 向任务处理器提供参数、日志、结果和制品目录。
	Context = task.Context
	// TaskLogger 定义任务日志写入和关闭行为。
	TaskLogger = task.Logger
	// ArtifactRoots 描述 Executor 为任务准备的默认层、兼容聚合目录和具名层。
	ArtifactRoots = task.ArtifactRoots
	// PreparedArtifacts 是一次任务独占的制品运行现场。
	PreparedArtifacts = artifactport.PreparedArtifacts
	// ArtifactPreparer 定义 Executor 可选的制品本地物化能力。
	ArtifactPreparer = artifactport.Preparer
)

// NewContext 创建任务处理器上下文。
func NewContext(options ContextOptions) *Context {
	return task.NewContext(options)
}
