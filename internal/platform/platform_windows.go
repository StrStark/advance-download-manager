//go:build windows

package platform

import (
	"os/exec"
	"syscall"
	"unsafe"
)

func Open(path string) error {
	return exec.Command("rundll32", "url.dll,FileProtocolHandler", path).Start()
}

func Reveal(path string) error {
	return exec.Command("explorer", "/select,", path).Start()
}

// Notify is a no-op on Windows for now; toast notifications need an AppUserModelID
// and are best added together with the installer.
func Notify(title, body string) {}

var procGetDiskFreeSpaceEx = syscall.NewLazyDLL("kernel32.dll").NewProc("GetDiskFreeSpaceExW")

func diskFree(dir string) (uint64, error) {
	p, err := syscall.UTF16PtrFromString(dir)
	if err != nil {
		return 0, err
	}
	var free uint64
	r, _, e := procGetDiskFreeSpaceEx.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&free)), 0, 0)
	if r == 0 {
		return 0, e
	}
	return free, nil
}
