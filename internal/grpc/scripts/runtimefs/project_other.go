//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package runtimefs

import "os"

func canHardlinkArtifact(os.FileInfo) bool {
	return false
}
