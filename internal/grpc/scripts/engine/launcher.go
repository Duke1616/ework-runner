package engine

import "os/exec"

// ProcessLauncher 在统一的安全边界内启动脚本子进程。
type ProcessLauncher interface {
	Start(command *exec.Cmd) error
}

type hostProcessLauncher struct{}

// NewHostProcessLauncher 创建沿用 Executor 身份的进程启动器。
func NewHostProcessLauncher() ProcessLauncher {
	return hostProcessLauncher{}
}

func (hostProcessLauncher) Start(command *exec.Cmd) error {
	return command.Start()
}
