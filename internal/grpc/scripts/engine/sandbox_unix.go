//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package engine

import (
	"fmt"
	"os/exec"
	"syscall"
)

type credentialProcessLauncher struct {
	uid uint32
	gid uint32
}

// NewCredentialProcessLauncher 创建使用指定非特权身份的进程启动器。
func NewCredentialProcessLauncher(uid, gid uint32) (ProcessLauncher, error) {
	if uid == 0 || gid == 0 {
		return nil, fmt.Errorf("脚本进程 UID/GID 必须是非 root 身份")
	}
	return credentialProcessLauncher{uid: uid, gid: gid}, nil
}

func (l credentialProcessLauncher) Start(command *exec.Cmd) error {
	if command.SysProcAttr == nil {
		command.SysProcAttr = &syscall.SysProcAttr{}
	}
	command.SysProcAttr.Credential = &syscall.Credential{
		Uid: l.uid, Gid: l.gid, Groups: []uint32{l.gid},
	}
	return command.Start()
}
