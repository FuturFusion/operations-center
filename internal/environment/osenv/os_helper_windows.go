//go:build windows

package osenv

import (
	"golang.org/x/sys/windows"
)

func GetStdinFd() int {
	return int(windows.Stdin)
}
