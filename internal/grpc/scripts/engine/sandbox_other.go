//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package engine

import (
	"fmt"
	"os/exec"
)

func configureCommandSandbox(_ *exec.Cmd, sandbox Sandbox) error {
	if sandbox.Enabled {
		return fmt.Errorf("当前操作系统不支持脚本身份降权")
	}
	return nil
}
