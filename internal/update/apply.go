package update

import (
	"errors"
	"io"
	"os"
	"path/filepath"
)

// ErrManual means this install can't update itself; show instructions.
var ErrManual = errors.New("this installation is updated outside the app")

// replaceBinary swaps the running executable for newPath (same directory,
// atomic rename), keeping it executable.
func replaceBinary(newPath string) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if p, err := filepath.EvalSymlinks(exe); err == nil {
		exe = p
	}
	tmp := exe + ".new"
	src, err := os.Open(newPath)
	if err != nil {
		return "", err
	}
	defer src.Close()
	dst, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		os.Remove(tmp)
		return "", err
	}
	if err := dst.Close(); err != nil {
		return "", err
	}
	return exe, os.Rename(tmp, exe)
}
