// Package executor 定义轻量的任务处理器和执行上下文契约。
package executor

import "github.com/Duke1616/etask/sdk/executor/internal/task"

type (
	// Variable 描述传给任务处理器的一个变量。
	Variable = task.Variable
	// VariableSet 表示一次执行提供的完整变量快照。
	VariableSet = task.VariableSet
	// Parameter 描述任务处理器支持的一个参数。
	Parameter = task.Parameter
	// ParameterRole 描述参数在执行协议中的语义用途。
	ParameterRole = task.ParameterRole
	// Binding 定义运行阶段的参数绑定行为。
	Binding = task.Binding
	// BindingOption 描述参数绑定的前端配置和可选解析函数。
	BindingOption = task.BindingOption
	// TaskHandler 定义 Executor 可以调度的一类任务。
	TaskHandler = task.TaskHandler
	// ProgramHandler 声明 Handler 支持的程序来源类型。
	ProgramHandler = task.ProgramHandler
	// HandlerMeta 是处理器展示和注册元数据。
	HandlerMeta = task.HandlerMeta
)

const (
	// ParameterRoleVariables 表示参数承载统一变量集合。
	ParameterRoleVariables = task.ParameterRoleVariables
)
