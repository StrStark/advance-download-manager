//go:build !(linux && !android) && !windows && !darwin

package autostart

func Enabled() bool     { return false }
func Set(on bool) error { return ErrUnsupported }
