//go:build darwin

package autostart

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
)

// macOS: a LaunchAgent that runs ADM at login.
func plistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", "io.github.adm.plist")
}

func Enabled() bool {
	_, err := os.Stat(plistPath())
	return err == nil
}

func Set(on bool) error {
	p := plistPath()
	if !on {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	exe, err := executable()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key><string>io.github.adm</string>
	<key>ProgramArguments</key>
	<array><string>%s</string><string>%s</string></array>
	<key>RunAtLoad</key><true/>
	<key>ProcessType</key><string>Interactive</string>
</dict>
</plist>
`, html.EscapeString(exe), BackgroundFlag)
	return os.WriteFile(p, []byte(plist), 0o644)
}
