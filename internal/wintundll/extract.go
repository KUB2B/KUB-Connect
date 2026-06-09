// Package wintundll bundles the Wintun driver DLL that tun2socks needs on
// Windows to create a TUN adapter. The DLL is embedded in the binary and
// extracted to a stable on-disk directory at runtime; the Windows build then
// pre-loads it by absolute path so the bare-name LoadLibraryEx call inside the
// wintun module resolves to our copy (Windows matches an already-loaded module
// by base name regardless of search path).
//
// Without this, the wintun module's LoadLibraryEx("wintun.dll", ...) searches
// only the executable directory and System32; a missing DLL makes tun2socks
// call log.Fatalf, which terminates the whole process.
package wintundll

import (
	"fmt"
	"os"
	"path/filepath"
)

// DefaultDir returns the stable cache directory for the extracted DLL.
func DefaultDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "kub-connect"), nil
}

// extract writes data to destDir/name, returning the full path. The file is
// written only when missing or size-mismatched, matching the geoassets policy
// so repeated launches don't rewrite the DLL needlessly.
func extract(data []byte, destDir, name string) (string, error) {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", destDir, err)
	}
	dst := filepath.Join(destDir, name)
	if fi, err := os.Stat(dst); err == nil && fi.Size() == int64(len(data)) {
		return dst, nil // already present
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", dst, err)
	}
	return dst, nil
}
