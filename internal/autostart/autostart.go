// Package autostart registers ADM to start when the user logs in. It only
// acts when the user turns the option on.
package autostart

import (
	"errors"
	"os"
	"path/filepath"
)

// ErrUnsupported is returned on platforms without login items.
var ErrUnsupported = errors.New("starting at login is not supported on this platform")

// BackgroundFlag makes ADM start minimised (used for login starts).
const BackgroundFlag = "--background"

// executable returns the path to launch: the real binary, with symlinks
// resolved, or "adm" when installed system-wide.
func executable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if p, err := filepath.EvalSymlinks(exe); err == nil {
		exe = p
	}
	return exe, nil
}
