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
	// ArtifactRoots 描述 Executor 为任务准备的默认层和具名依赖层。
	ArtifactRoots = task.ArtifactRoots
	// Program 描述 Executor 为 Handler 准备好的完整程序输入。
	Program = task.Program
	// ProgramKind 描述程序形态。
	ProgramKind = task.ProgramKind
	// InlineProgram 描述内联程序代码。
	InlineProgram = task.InlineProgram
	// ProjectProgram 描述已准备项目的根目录和入口。
	ProjectProgram = task.ProjectProgram
	// ProgressReporter 定义任务进度的运行环境上报能力。
	ProgressReporter = task.ProgressReporter
)

const (
	ProgramKindInline  = task.ProgramInline
	ProgramKindProject = task.ProgramProject
)

// NewContext 创建任务处理器上下文。
func NewContext(options ContextOptions) *Context {
	return task.NewContext(options)
}
