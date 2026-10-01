//go:build darwin

package browser

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

func register(exe string) error {
	home, _ := os.UserHomeDir()
	support := filepath.Join(home, "Library", "Application Support")
	var errs []error
	for _, b := range []string{"Google/Chrome", "Google/Chrome Beta", "Chromium", "BraveSoftware/Brave-Browser", "Microsoft Edge", "Vivaldi"} {
		errs = append(errs, writeManifest(filepath.Join(support, b, "NativeMessagingHosts", HostName+".json"), chromeManifest(exe)))
	}
	errs = append(errs, writeManifest(filepath.Join(support, "Mozilla", "NativeMessagingHosts", HostName+".json"), firefoxManifest(exe)))
	return errors.Join(errs...)
}

func detach(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
