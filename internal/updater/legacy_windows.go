//go:build windows

package updater

import "golang.org/x/sys/windows"

// LegacyWindows reports whether the host predates Windows 10 (i.e. Windows
// 7/8/8.1), which need the separately-built "windows7" installer — the mainline
// installer ships a Go 1.21+ binary that crashes at startup on those systems.
// Uses RtlGetVersion so the result is accurate without an application manifest.
func LegacyWindows() bool {
	return windows.RtlGetVersion().MajorVersion < 10
}
