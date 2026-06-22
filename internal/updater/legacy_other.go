//go:build !windows

package updater

// LegacyWindows is always false off Windows; the in-app updater only ships
// Windows installers.
func LegacyWindows() bool { return false }
