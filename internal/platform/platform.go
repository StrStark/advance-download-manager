// Package platform wraps the few OS-specific operations ADM needs: opening
// files, revealing them in the file manager, notifications, disk space and
// the per-user data directory.
package platform

import (
	"os"
	"path/filepath"
	"runtime"
)

// DataDir is where ADM keeps its state:
//   - Linux:   $XDG_DATA_HOME/adm (~/.local/share/adm)
//   - macOS:   ~/Library/Application Support/ADM
//   - Windows: %AppData%\ADM
//
// ADM_DATA_DIR overrides it (used by the container image).
func DataDir() string {
	if d := os.Getenv("ADM_DATA_DIR"); d != "" {
		return d
	}
	if runtime.GOOS == "linux" {
		base := os.Getenv("XDG_DATA_HOME")
		if base == "" {
			home, _ := os.UserHomeDir()
			base = filepath.Join(home, ".local", "share")
		}
		return filepath.Join(base, "adm")
	}
	base, err := os.UserConfigDir()
	if err != nil {
		home, _ := os.UserHomeDir()
		base = home
	}
	return filepath.Join(base, "ADM")
}

// DiskFree reports free bytes on the filesystem holding dir, walking up to
// the nearest existing ancestor (the target folder may not exist yet).
func DiskFree(dir string) (uint64, error) {
	d := dir
	for {
		if _, err := os.Stat(d); err == nil {
			return diskFree(d)
		}
		parent := filepath.Dir(d)
		if parent == d {
			return diskFree(d)
		}
		d = parent
	}
}
