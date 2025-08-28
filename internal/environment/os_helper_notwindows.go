//go:build linux || darwin || freebsd || openbsd

package environment

import "golang.org/x/sys/unix"

func GetStdinFd() int {
	return unix.Stdin
}
