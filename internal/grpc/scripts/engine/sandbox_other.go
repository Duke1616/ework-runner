//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package engine

import "fmt"

// NewCredentialProcessLauncher 在不支持身份降权的平台返回装配错误。
func NewCredentialProcessLauncher(uint32, uint32) (ProcessLauncher, error) {
	return nil, fmt.Errorf("当前操作系统不支持脚本身份降权")
}
