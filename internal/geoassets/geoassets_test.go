package geoassets

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestSyncWritesNamedFiles(t *testing.T) {
	fsys := fstest.MapFS{
		"data/geoip.dat":   {Data: []byte("ip-data")},
		"data/geosite.dat": {Data: []byte("site-data")},
	}
	dst := t.TempDir()

	if err := Sync(fsys, "data", dst, "", []string{"geoip.dat", "geosite.dat"}); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dst, "geoip.dat"))
	if err != nil {
		t.Fatalf("read geoip: %v", err)
	}
	if string(got) != "ip-data" {
		t.Errorf("geoip.dat = %q, want %q", got, "ip-data")
	}
	got, err = os.ReadFile(filepath.Join(dst, "geosite.dat"))
	if err != nil {
		t.Fatalf("read geosite: %v", err)
	}
	if string(got) != "site-data" {
		t.Errorf("geosite.dat = %q, want %q", got, "site-data")
	}
}

func TestSyncSkipsWhenSizeMatches(t *testing.T) {
	fsys := fstest.MapFS{
		"data/geoip.dat": {Data: []byte("12345")},
	}
	dst := t.TempDir()
	// Pre-existing file, same size, different content: Sync must not rewrite.
	existing := filepath.Join(dst, "geoip.dat")
	if err := os.WriteFile(existing, []byte("XXXXX"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Sync(fsys, "data", dst, "", []string{"geoip.dat"}); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	got, err := os.ReadFile(existing)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "XXXXX" {
		t.Errorf("file rewritten: got %q, want untouched %q", got, "XXXXX")
	}
}

func TestSyncRewritesWhenSizeDiffers(t *testing.T) {
	fsys := fstest.MapFS{
		"data/geoip.dat": {Data: []byte("new-longer-data")},
	}
	dst := t.TempDir()
	stale := filepath.Join(dst, "geoip.dat")
	if err := os.WriteFile(stale, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Sync(fsys, "data", dst, "", []string{"geoip.dat"}); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	got, err := os.ReadFile(stale)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new-longer-data" {
		t.Errorf("file = %q, want rewritten %q", got, "new-longer-data")
	}
}

func TestSyncMissingEmbeddedFileErrors(t *testing.T) {
	fsys := fstest.MapFS{}
	dst := t.TempDir()

	if err := Sync(fsys, "data", dst, "", []string{"geoip.dat"}); err == nil {
		t.Fatal("expected error for missing embedded file, got nil")
	}
}

func TestSyncRewritesSameSizeFileOnVersionChange(t *testing.T) {
	dst := t.TempDir()
	path := filepath.Join(dst, "geoip.dat")
	read := func() string {
		t.Helper()
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	sync := func(data, version string) {
		t.Helper()
		fsys := fstest.MapFS{"data/geoip.dat": {Data: []byte(data)}}
		if err := Sync(fsys, "data", dst, version, []string{"geoip.dat"}); err != nil {
			t.Fatalf("Sync: %v", err)
		}
	}

	sync("AAAAA", "v1")
	// Same version, same size: the cache is trusted (no 30MB rewrite per start).
	if err := os.WriteFile(path, []byte("XXXXX"), 0o644); err != nil {
		t.Fatal(err)
	}
	sync("AAAAA", "v1")
	if got := read(); got != "XXXXX" {
		t.Errorf("same version rewrote the file: %q", got)
	}
	// New version ships a database of the same size: it must replace the old one.
	sync("BBBBB", "v2")
	if got := read(); got != "BBBBB" {
		t.Errorf("after a version change got %q, want the new contents", got)
	}
}
