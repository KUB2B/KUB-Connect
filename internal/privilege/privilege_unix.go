//go:build linux || darwin

package privilege

import "os"

// IsElevated reports whether the effective user is root.
func IsElevated() bool {
	return os.Geteuid() == 0
}
