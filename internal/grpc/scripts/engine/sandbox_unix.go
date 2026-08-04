//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package engine

import (
	"fmt"
	"os/exec"
	"syscall"
)

func configureCommandSandbox(command *exec.Cmd, sandbox Sandbox) error {
	if !sandbox.Enabled {
		return nil
	}
	if command.SysProcAttr == nil {
		command.SysProcAttr = &syscall.SysProcAttr{}
	}
	command.SysProcAttr.Credential = &syscall.Credential{
		Uid: sandbox.UID, Gid: sandbox.GID, Groups: []uint32{sandbox.GID},
	}
	if command.SysProcAttr.Credential.Uid == 0 || command.SysProcAttr.Credential.Gid == 0 {
		return fmt.Errorf("脚本沙箱拒绝使用 root 身份")
	}
	return nil
}
