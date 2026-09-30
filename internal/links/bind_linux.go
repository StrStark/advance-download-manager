//go:build linux && !android

package links

import "syscall"

// Linux: SO_BINDTODEVICE makes the kernel route the socket through that
// interface (unprivileged since Linux 5.7).
func bind(fd uintptr, network string, l Link) error {
	return syscall.BindToDevice(int(fd), l.Name)
}
