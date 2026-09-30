//go:build android && !cgo

package links

func bind(fd uintptr, network string, l Link) error { return ErrUnsupported }
