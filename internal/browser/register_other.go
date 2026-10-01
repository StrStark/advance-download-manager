//go:build !(linux && !android) && !darwin && !windows

package browser

import (
	"errors"
	"os/exec"
)

func register(exe string) error {
	return errors.New("browser integration is not supported on this platform")
}

func detach(cmd *exec.Cmd) {}
