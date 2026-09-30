//go:build linux || freebsd || openbsd || netbsd

package platform

import (
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

func Open(path string) error { return exec.Command("xdg-open", path).Start() }

// Reveal asks the file manager (via the freedesktop FileManager1 D-Bus API)
// to highlight the file, falling back to opening its folder.
func Reveal(path string) error {
	uri := (&url.URL{Scheme: "file", Path: path}).String()
	err := exec.Command("gdbus", "call", "--session",
		"--dest", "org.freedesktop.FileManager1",
		"--object-path", "/org/freedesktop/FileManager1",
		"--method", "org.freedesktop.FileManager1.ShowItems",
		"['"+strings.ReplaceAll(uri, "'", "%27")+"']", "").Run()
	if err != nil {
		return exec.Command("xdg-open", filepath.Dir(path)).Start()
	}
	return nil
}

func Notify(title, body string) {
	if _, err := exec.LookPath("notify-send"); err == nil {
		_ = exec.Command("notify-send", "--app-name=ADM", "--icon=adm", title, body).Start()
	}
}

func diskFree(dir string) (uint64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return 0, err
	}
	return st.Bavail * uint64(st.Bsize), nil
}
