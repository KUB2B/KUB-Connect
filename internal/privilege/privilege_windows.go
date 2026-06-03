//go:build windows

package privilege

import "golang.org/x/sys/windows"

// IsElevated reports whether the process token is elevated (Administrator).
func IsElevated() bool {
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token); err != nil {
		return false
	}
	defer token.Close()
	return token.IsElevated()
}
