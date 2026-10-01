//go:build linux && !android

package autostart

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Linux: an XDG autostart entry in ~/.config/autostart.
func entryPath() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "autostart", "adm.desktop")
}

func Enabled() bool {
	b, err := os.ReadFile(entryPath())
	return err == nil && !strings.Contains(string(b), "Hidden=true")
}

func Set(on bool) error {
	p := entryPath()
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
	entry := fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=ADM
Comment=Download manager
Exec="%s" %s
Icon=adm
Terminal=false
X-GNOME-Autostart-enabled=true
X-GNOME-Autostart-Delay=3
`, exe, BackgroundFlag)
	return os.WriteFile(p, []byte(entry), 0o644)
}
