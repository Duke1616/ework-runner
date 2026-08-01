package event

import executionevent "github.com/Duke1616/etask/internal/event/execution"

type (
	// ExecuteCommand 是 Agent 接收的不可变执行命令。
	ExecuteCommand = executionevent.Command
	// ControlCommand 是 Agent 接收的终止控制命令。
	ControlCommand = executionevent.ControlCommand
)
