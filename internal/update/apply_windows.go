//go:build windows

package update

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// Apply runs the installer silently once ADM has exited, then starts ADM
// again. The installer asks for administrator rights (UAC).
func Apply(path string, k Kind, args []string) error {
	if k != Windows {
		return ErrManual
	}
	exe, _ := os.Executable()
	if p, err := filepath.EvalSymlinks(exe); err == nil {
		exe = p
	}
	// ping is the classic "sleep" that works without a console; it gives ADM
	// time to quit so the installer can replace adm.exe.
	line := fmt.Sprintf(`/C ping -n 3 127.0.0.1 >nul & start "" /wait "%s" /S & start "" "%s"`, path, exe)
	cmd := exec.Command(os.Getenv("ComSpec"))
	if cmd.Path == "" {
		cmd = exec.Command("cmd.exe")
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CmdLine:       `cmd.exe ` + line,
		HideWindow:    true,
		CreationFlags: 0x00000008 | syscall.CREATE_NEW_PROCESS_GROUP, // DETACHED_PROCESS
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
