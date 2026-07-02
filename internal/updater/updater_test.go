package updater

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestIsNewer(t *testing.T) {
	cases := []struct {
		current, latest string
		want            bool
	}{
		// normal upgrade path
		{"v1.0.11", "v1.0.12", true},
		{"v1.0.12", "v1.0.12", false},
		// current ahead of latest must NOT prompt (the bug we fixed)
		{"v1.0.13", "v1.0.12", false},
		{"v2.0.0", "v1.9.9", false},
		// minor / major bumps
		{"v1.0.0", "v1.1.0", true},
		{"v1.9.0", "v2.0.0", true},
		// missing leading v on either side
		{"1.0.11", "1.0.12", true},
		{"1.0.12", "v1.0.12", false},
		// dev / empty are always silent
		{"dev", "v1.0.12", false},
		{"", "v1.0.12", false},
		{"v1.0.11", "", false},
		// prerelease ordering: stable > prerelease of same version
		{"v1.0.0-rc1", "v1.0.0", true},
		{"v1.0.0", "v1.0.0-rc1", false},
		// unparseable latest falls back to inequality
		{"v1.0.11", "nightly", true},
		{"nightly", "nightly", false},
	}
	for _, c := range cases {
		if got := IsNewer(c.current, c.latest); got != c.want {
			t.Errorf("IsNewer(%q, %q) = %v, want %v", c.current, c.latest, got, c.want)
		}
	}
}

func TestPickInstaller(t *testing.T) {
	// A v1.2.0+ release carries both installers. The win7 asset is listed FIRST
	// here on purpose: GitHub asset order is not contractual, so selection must
	// be driven by the running OS, not by array position.
	dual := Release{Assets: []Asset{
		{Name: "kub-connect-v1.2.0-linux-amd64", URL: "https://x/linux", Size: 5},
		{Name: "kub-connect-v1.2.0-windows7-amd64-installer.exe", URL: "https://x/win7", Size: 20},
		{Name: "kub-connect-v1.2.0-windows-amd64-installer.exe", URL: "https://x/win10", Size: 21},
	}}

	// Modern Windows must get the mainline installer, never the win7 one.
	if a, ok := PickInstaller(dual, false); !ok || a.URL != "https://x/win10" {
		t.Errorf("PickInstaller(modern) = %+v ok=%v, want win10 asset", a, ok)
	}
	// Win7/8 must get the windows7 installer (the mainline binary crashes there).
	if a, ok := PickInstaller(dual, true); !ok || a.URL != "https://x/win7" {
		t.Errorf("PickInstaller(legacy) = %+v ok=%v, want win7 asset", a, ok)
	}

	// Old single-installer release (pre-v1.2.0): both OSes fall back to it.
	single := Release{Assets: []Asset{
		{Name: "kub-connect-v1.1.3-windows-amd64-installer.exe", URL: "https://x/old", Size: 20},
	}}
	if a, ok := PickInstaller(single, false); !ok || a.URL != "https://x/old" {
		t.Errorf("PickInstaller(modern, old release) = %+v ok=%v, want old asset", a, ok)
	}
	if a, ok := PickInstaller(single, true); !ok || a.URL != "https://x/old" {
		t.Errorf("PickInstaller(legacy, old release) = %+v ok=%v, want old asset fallback", a, ok)
	}

	if _, ok := PickInstaller(Release{Assets: []Asset{{Name: "notes.txt"}}}, false); ok {
		t.Error("PickInstaller: expected ok=false when no installer asset")
	}

	// Partial release with only the win7 asset (e.g. `make windows7` published
	// without `make windows`): modern host must NOT fall back to it.
	win7Only := Release{Assets: []Asset{
		{Name: "kub-connect-v1.2.2-windows7-amd64-installer.exe", URL: "https://x/win7"},
	}}
	if _, ok := PickInstaller(win7Only, false); ok {
		t.Error("PickInstaller(modern, win7-only release): expected ok=false, got a match")
	}
	if a, ok := PickInstaller(win7Only, true); !ok || a.URL != "https://x/win7" {
		t.Errorf("PickInstaller(legacy, win7-only release) = %+v ok=%v, want win7 asset", a, ok)
	}
}

func TestDownload(t *testing.T) {
	body := []byte("hello-installer-bytes")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(body)
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "out.exe")
	a := Asset{Name: "x-installer.exe", URL: srv.URL, Size: int64(len(body))}

	var lastDone, lastTotal int64
	var calls int
	err := Download(context.Background(), a, dst, func(done, total int64) {
		if done < lastDone {
			t.Errorf("progress done decreased: %d after %d", done, lastDone)
		}
		lastDone, lastTotal = done, total
		calls++
	})
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	got, _ := os.ReadFile(dst)
	if string(got) != string(body) {
		t.Errorf("downloaded content = %q, want %q", got, body)
	}
	if calls == 0 {
		t.Error("progress callback never fired")
	}
	if lastDone != int64(len(body)) || lastTotal != int64(len(body)) {
		t.Errorf("final progress = %d/%d, want %d/%d", lastDone, lastTotal, len(body), len(body))
	}
}

func TestDownloadSizeMismatchRemovesFile(t *testing.T) {
	body := []byte("short")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(body)
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "out.exe")
	a := Asset{Name: "x-installer.exe", URL: srv.URL, Size: int64(len(body)) + 1} // wrong size

	if err := Download(context.Background(), a, dst, nil); err == nil {
		t.Fatal("Download: expected size-mismatch error, got nil")
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Error("Download: partial file should be removed on size mismatch")
	}
}
