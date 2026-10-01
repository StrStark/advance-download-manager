//go:build windows

package autostart

import (
	"golang.org/x/sys/windows/registry"
)

// Windows: a value under HKCU\...\CurrentVersion\Run.
const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`
const valueName = "ADM"

func Enabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	_, _, err = k.GetStringValue(valueName)
	return err == nil
}

func Set(on bool) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if !on {
		if err := k.DeleteValue(valueName); err != nil && err != registry.ErrNotExist {
			return err
		}
		return nil
	}
	exe, err := executable()
	if err != nil {
		return err
	}
	return k.SetStringValue(valueName, `"`+exe+`" `+BackgroundFlag)
}
