//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package runtimefs

import (
	"os"
	"syscall"
)

func canHardlinkArtifact(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == 0 && info.Mode().Perm()&0o222 == 0 && info.Mode().Perm()&0o004 != 0
}
