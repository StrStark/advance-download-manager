//go:build (linux && !android) || darwin

package update

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// Apply starts installing the downloaded update. The caller should quit the
// app right after; the update relaunches ADM when it's done.
func Apply(path string, k Kind, args []string) error {
	switch k {
	case Deb:
		// pkexec shows the system password prompt; apt resolves dependencies.
		if _, err := exec.LookPath("pkexec"); err != nil {
			return spawn("xdg-open", path)
		}
		script := `sleep 1; pkexec apt-get install -y "$1" && exec /usr/bin/adm`
		return spawn("sh", "-c", script, "adm-update", path)
	case Portable:
		exe, err := replaceBinary(path)
		if err != nil {
			return fmt.Errorf("could not replace %s: %w", filepath.Base(os.Args[0]), err)
		}
		return spawn(exe, args...)
	case MacOS:
		exe, _ := os.Executable()
		bundle := exe
		for i := 0; i < 3 && !strings.HasSuffix(bundle, ".app"); i++ {
			bundle = filepath.Dir(bundle)
		}
		if !strings.HasSuffix(bundle, ".app") {
			return spawn("open", path) // not running from a bundle: let the user drag it
		}
		script := fmt.Sprintf(`while kill -0 %d 2>/dev/null; do sleep 0.3; done
mnt=$(mktemp -d)
hdiutil attach -nobrowse -quiet -mountpoint "$mnt" "$1" || exit 1
app=$(ls -d "$mnt"/*.app | head -1)
rm -rf "$2.old" && mv "$2" "$2.old" && ditto "$app" "$2" && rm -rf "$2.old" || { rm -rf "$2"; mv "$2.old" "$2"; }
hdiutil detach -quiet "$mnt"
xattr -dr com.apple.quarantine "$2" 2>/dev/null
open "$2"`, os.Getpid())
		return spawn("sh", "-c", script, "adm-update", path, bundle)
	}
	return ErrManual
}

func spawn(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
