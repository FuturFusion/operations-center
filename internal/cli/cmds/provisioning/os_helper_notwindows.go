//go:build linux || darwin || freebsd || openbsd

package provisioning

import "golang.org/x/sys/unix"

func getStdinFd() int {
	return unix.Stdin
}
