//go:build android && cgo

package links

/*
#cgo LDFLAGS: -landroid
#include <android/multinetwork.h>
#include <errno.h>

static int adm_bind(int fd, long long handle) {
	return android_setsocknetwork((net_handle_t)handle, fd) == 0 ? 0 : errno;
}
*/
import "C"

import (
	"errors"
	"syscall"
)

// Android: networks are selected by handle, not interface name, because the
// OS routes with per-network tables (android_setsocknetwork, API 23+).
func bind(fd uintptr, network string, l Link) error {
	if l.Handle == 0 {
		return errors.New("link has no Android network handle")
	}
	if e := C.adm_bind(C.int(fd), C.longlong(l.Handle)); e != 0 {
		return syscall.Errno(e)
	}
	return nil
}
