//go:build windows

package browser

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/windows/registry"
)

// Windows: manifests live in %AppData%\ADM; each browser finds them through
// a registry key under HKCU.
func register(exe string) error {
	base, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(base, "ADM")
	chromeJSON := filepath.Join(dir, "native-host-chrome.json")
	firefoxJSON := filepath.Join(dir, "native-host-firefox.json")
	errs := []error{writeManifest(chromeJSON, chromeManifest(exe)), writeManifest(firefoxJSON, firefoxManifest(exe))}
	for _, key := range []string{
		`Software\Google\Chrome\NativeMessagingHosts\`,
		`Software\Chromium\NativeMessagingHosts\`,
		`Software\BraveSoftware\Brave-Browser\NativeMessagingHosts\`,
		`Software\Microsoft\Edge\NativeMessagingHosts\`,
		`Software\Vivaldi\NativeMessagingHosts\`,
	} {
		errs = append(errs, setDefault(key+HostName, chromeJSON))
	}
	errs = append(errs, setDefault(`Software\Mozilla\NativeMessagingHosts\`+HostName, firefoxJSON))
	return errors.Join(errs...)
}

func setDefault(path, value string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue("", value)
}

func detach(cmd *exec.Cmd) {
	const detachedProcess = 0x00000008
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: detachedProcess | syscall.CREATE_NEW_PROCESS_GROUP}
}
