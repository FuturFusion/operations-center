//go:build linux || darwin || freebsd || openbsd

package osenv

import "golang.org/x/sys/unix"

func GetStdinFd() int {
	return unix.Stdin
}
