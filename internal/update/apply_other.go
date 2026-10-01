//go:build !((linux && !android) || darwin || windows)

package update

// Apply is handled by the host app on Android, and manually elsewhere.
func Apply(path string, k Kind, args []string) error { return ErrManual }
