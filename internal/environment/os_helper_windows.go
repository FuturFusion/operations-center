//go:build windows

package environment

import (
	"golang.org/x/sys/windows"
)

func GetStdinFd() int {
	return int(windows.Stdin)
}
