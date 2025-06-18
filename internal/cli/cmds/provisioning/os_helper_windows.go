//go:build windows

package provisioning

import (
	"golang.org/x/sys/windows"
)

func getStdinFd() int {
	return int(windows.Stdin)
}
