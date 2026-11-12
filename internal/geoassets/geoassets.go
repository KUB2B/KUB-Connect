// Package geoassets extracts embedded geo data files (geoip.dat, geosite.dat)
// to a stable on-disk directory so xray-core can load them via
// XRAY_LOCATION_ASSET. Files are rewritten once per app version, avoiding a
// ~30MB rewrite on every launch.
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

// versionFile records which app version populated destDir.
const versionFile = ".version"

// Sync copies the named files from fsys (under srcDir) into destDir. Files are
// rewritten when the cache was populated by a different app version (a size
// comparison alone misses an updated database of the same size, and never
// repairs a damaged one), and otherwise only when missing or size-mismatched,
// so a normal start does not rewrite ~30MB. An empty version disables the
// version check. Returns the first error encountered.
func Sync(fsys fs.FS, srcDir, destDir, version string, names []string) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", destDir, err)
	}
	marker := filepath.Join(destDir, versionFile)
	prev, _ := os.ReadFile(marker)
	sameVersion := version != "" && string(prev) == version
	for _, name := range names {
		src := path.Join(srcDir, name)
		// Stat first: reading an embedded file copies it to the heap (~30MB
		// for the geo databases), which the common "already present" path
		// does not need.
		srcInfo, err := fs.Stat(fsys, src)
		if err != nil {
			return fmt.Errorf("read embedded %s: %w", src, err)
		}
		dst := filepath.Join(destDir, name)
		if fi, err := os.Stat(dst); err == nil && fi.Size() == srcInfo.Size() && (sameVersion || version == "") {
			continue // already present
		}
		data, err := fs.ReadFile(fsys, src)
		if err != nil {
			return fmt.Errorf("read embedded %s: %w", src, err)
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", dst, err)
		}
	}
	if version != "" && !sameVersion {
		if err := os.WriteFile(marker, []byte(version), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", marker, err)
		}
	}
	return nil
}
