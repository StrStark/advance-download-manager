//go:build !linux && !android && !windows && !darwin

package links

func bind(fd uintptr, network string, l Link) error { return ErrUnsupported }
