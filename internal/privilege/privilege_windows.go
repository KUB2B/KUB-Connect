//go:build windows

package privilege

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// IsElevated reports whether the process token is elevated (Administrator).
func IsElevated() bool {
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token); err != nil {
		return false
	}
	defer token.Close()
	return token.IsElevated()
}

var (
	shell32          = windows.NewLazySystemDLL("shell32.dll")
	procShellExecute = shell32.NewProc("ShellExecuteW")
)

// RelaunchElevated starts a new elevated instance of the current executable
// (same args) via the UAC "runas" verb. The caller quits the current instance
// after a nil return. Returns ErrElevationDeclined if the user dismisses UAC.
func RelaunchElevated() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}
	verb, _ := syscall.UTF16PtrFromString("runas")
	file, _ := syscall.UTF16PtrFromString(exe)
	dir, _ := syscall.UTF16PtrFromString(filepath.Dir(exe))

	var paramsPtr *uint16
	if params := joinArgs(os.Args[1:]); params != "" {
		paramsPtr, _ = syscall.UTF16PtrFromString(params)
	}

	const swShowNormal = 1
	ret, _, _ := procShellExecute.Call(
		0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(file)),
		uintptr(unsafe.Pointer(paramsPtr)),
		uintptr(unsafe.Pointer(dir)),
		uintptr(swShowNormal),
	)
	// ShellExecuteW returns a value > 32 on success; <= 32 is an SE_ERR_* code.
	if ret <= 32 {
		// Dismissing the UAC prompt surfaces as SE_ERR_ACCESSDENIED (5).
		if ret == 5 {
			return ErrElevationDeclined
		}
		return fmt.Errorf("ShellExecuteW failed (code %d)", ret)
	}
	return nil
}

// joinArgs builds a Windows command-line parameter string, quoting args that
// contain whitespace or quotes. This build passes no runtime args, so the
// result is normally empty; quoting is defensive.
func joinArgs(args []string) string {
	out := make([]string, len(args))
	for i, a := range args {
		if strings.ContainsAny(a, " \t\"") {
			out[i] = `"` + strings.ReplaceAll(a, `"`, `\"`) + `"`
		} else {
			out[i] = a
		}
	}
	return strings.Join(out, " ")
}
