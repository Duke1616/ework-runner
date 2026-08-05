package executor

import "github.com/Duke1616/etask/sdk/executor/internal/task"

type (
	// TaskInfo 描述一次任务执行的只读身份信息。
	TaskInfo = task.TaskInfo
	// ContextOptions 描述创建任务上下文所需的输入和依赖。
	ContextOptions = task.ContextOptions
	// Context 向任务处理器提供参数、日志、结果和制品目录。
	Context = task.Context
	// ExecutionLogger 定义用户可见的任务执行日志写入和关闭行为。
	ExecutionLogger = task.ExecutionLogger
	// SystemLogger 定义 Executor 和 Handler 的结构化内部诊断日志端口。
	SystemLogger = task.SystemLogger
	// ArtifactRoots 描述 Executor 为任务准备的默认层和具名层。
	ArtifactRoots = task.ArtifactRoots
	// ProgressReporter 定义任务进度的运行环境上报能力。
	ProgressReporter = task.ProgressReporter
)

// NewContext 创建任务处理器上下文。
func NewContext(options ContextOptions) *Context {
	return task.NewContext(options)
}
