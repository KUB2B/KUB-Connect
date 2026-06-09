// Package geoassets extracts embedded geo data files (geoip.dat, geosite.dat)
// to a stable on-disk directory so xray-core can load them via
// XRAY_LOCATION_ASSET. Files are written only when missing or size-mismatched,
// avoiding a ~30MB rewrite on every launch.
package geoassets

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
)

// DefaultDir returns the stable cache directory for extracted geo assets.
func DefaultDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "kub-connect", "geo"), nil
}

// Sync copies the named files from fsys (under srcDir) into destDir. A file is
// written only when it is missing or its size differs from the embedded copy;
// matching sizes are treated as already present and skipped. Returns the first
// error encountered.
func Sync(fsys fs.FS, srcDir, destDir string, names []string) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", destDir, err)
	}
	for _, name := range names {
		src := path.Join(srcDir, name)
		data, err := fs.ReadFile(fsys, src)
		if err != nil {
			return fmt.Errorf("read embedded %s: %w", src, err)
		}
		dst := filepath.Join(destDir, name)
		if fi, err := os.Stat(dst); err == nil && fi.Size() == int64(len(data)) {
			continue // already present
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", dst, err)
		}
	}
	return nil
}
