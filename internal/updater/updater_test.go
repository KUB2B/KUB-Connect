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
	rel := Release{Assets: []Asset{
		{Name: "KUB-Connect.dmg", URL: "https://x/dmg", Size: 10},
		{Name: "kub-connect-amd64-installer.exe", URL: "https://x/exe", Size: 20},
	}}
	a, ok := PickInstaller(rel)
	if !ok {
		t.Fatal("PickInstaller: expected ok=true")
	}
	if a.Name != "kub-connect-amd64-installer.exe" || a.URL != "https://x/exe" {
		t.Errorf("PickInstaller picked wrong asset: %+v", a)
	}

	if _, ok := PickInstaller(Release{Assets: []Asset{{Name: "notes.txt"}}}); ok {
		t.Error("PickInstaller: expected ok=false when no installer asset")
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
