//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package scripts

func effectiveUID() int {
	return -1
}
