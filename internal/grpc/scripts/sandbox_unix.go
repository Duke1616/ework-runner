//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package scripts

import "os"

func effectiveUID() int {
	return os.Geteuid()
}
