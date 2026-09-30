//go:build darwin

package platform

import (
	"os/exec"
	"strings"
	"syscall"
)

func Open(path string) error { return exec.Command("open", path).Start() }

func Reveal(path string) error { return exec.Command("open", "-R", path).Start() }

func Notify(title, body string) {
	esc := func(s string) string { return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) }
	script := `display notification "` + esc(body) + `" with title "` + esc(title) + `"`
	_ = exec.Command("osascript", "-e", script).Start()
}

func diskFree(dir string) (uint64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return 0, err
	}
	return st.Bavail * uint64(st.Bsize), nil
}
