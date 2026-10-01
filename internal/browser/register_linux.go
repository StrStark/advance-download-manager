//go:build linux && !android

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
	cfg := os.Getenv("XDG_CONFIG_HOME")
	if cfg == "" {
		cfg = filepath.Join(home, ".config")
	}
	chromium := []string{
		"google-chrome", "google-chrome-beta", "google-chrome-unstable", "chromium",
		"BraveSoftware/Brave-Browser", "microsoft-edge", "vivaldi", "opera",
	}
	var errs []error
	for _, b := range chromium {
		errs = append(errs, writeManifest(filepath.Join(cfg, b, "NativeMessagingHosts", HostName+".json"), chromeManifest(exe)))
	}
	errs = append(errs, writeManifest(filepath.Join(home, ".mozilla", "native-messaging-hosts", HostName+".json"), firefoxManifest(exe)))
	// Snap packages of Firefox and Chromium look inside their own folders.
	if _, err := os.Stat(filepath.Join(home, "snap", "firefox")); err == nil {
		errs = append(errs, writeManifest(filepath.Join(home, "snap", "firefox", "common", ".mozilla", "native-messaging-hosts", HostName+".json"), firefoxManifest(exe)))
	}
	if _, err := os.Stat(filepath.Join(home, "snap", "chromium")); err == nil {
		errs = append(errs, writeManifest(filepath.Join(home, "snap", "chromium", "common", "chromium", "NativeMessagingHosts", HostName+".json"), chromeManifest(exe)))
	}
	return errors.Join(errs...)
}

func detach(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
