package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

const apiURL = "https://api.github.com/repos/KUB2B/KUB-Connect/releases/latest"

type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
	// Digest is the asset checksum GitHub computes on upload, "sha256:<hex>".
	// Download refuses an asset without it: the installer runs elevated.
	Digest string `json:"digest"`
}

type Release struct {
	TagName string  `json:"tag_name"`
	HTMLURL string  `json:"html_url"`
	Assets  []Asset `json:"assets"`
}

// PickInstaller returns the Windows installer asset matching the running OS.
// Since v1.2.0 a release carries two installers — "-windows-amd64-installer.exe"
// (mainline, Win10+) and "-windows7-amd64-installer.exe" (Win7/8). The mainline
// binary crashes on Win7, so legacy hosts MUST get the windows7 build; selection
// can't rely on asset order (GitHub doesn't guarantee it). When the exact match
// is absent (older single-installer releases), falls back to any "-installer.exe".
// Reports false if the release carries no installer at all.
func PickInstaller(rel Release, legacy bool) (Asset, bool) {
	want := "-windows-amd64-installer.exe"
	if legacy {
		want = "-windows7-amd64-installer.exe"
	}
	for _, a := range rel.Assets {
		if strings.HasSuffix(strings.ToLower(a.Name), want) {
			return a, true
		}
	}
	// Fallback for releases that predate the per-OS split. Still must not hand
	// a modern host the windows7 asset if that's all a partial release has.
	for _, a := range rel.Assets {
		name := strings.ToLower(a.Name)
		if !strings.HasSuffix(name, "-installer.exe") {
			continue
		}
		if !legacy && strings.Contains(name, "windows7") {
			continue
		}
		return a, true
	}
	return Asset{}, false
}

// CheckLatest fetches the latest GitHub release. Returns empty Release on error.
func CheckLatest() (Release, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return Release{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("github releases: HTTP %d", resp.StatusCode)
	}
	var r Release
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return Release{}, err
	}
	return r, nil
}

// IsNewer reports whether latestTag is a strictly higher version than current,
// compared by semantic versioning order. Returns false for dev builds so CI/dev
// users don't see spurious banners, and false when current is ahead of latest.
// Falls back to plain inequality if either value isn't valid semver.
func IsNewer(current, latestTag string) bool {
	if current == "dev" || current == "" || latestTag == "" {
		return false
	}
	cur := withV(current)
	lat := withV(latestTag)
	if !semver.IsValid(cur) || !semver.IsValid(lat) {
		// Unparseable tag — be conservative and only flag on difference.
		return strings.TrimPrefix(latestTag, "v") != strings.TrimPrefix(current, "v")
	}
	return semver.Compare(lat, cur) > 0
}

// progressWriter counts bytes written and reports cumulative progress.
type progressWriter struct {
	done, total int64
	cb          func(done, total int64)
}

func (w *progressWriter) Write(p []byte) (int, error) {
	n := len(p)
	w.done += int64(n)
	if w.cb != nil {
		w.cb(w.done, w.total)
	}
	return n, nil
}

// Download streams the asset to dst over HTTPS, calling progress(done, total)
// as bytes arrive (progress may be nil). total is the response Content-Length,
// falling back to a.Size when the server omits it. The file is kept only when
// its SHA-256 matches a.Digest; on any error, a size mismatch, or a missing or
// wrong digest the partial file is removed.
func Download(ctx context.Context, a Asset, dst string, progress func(done, total int64)) error {
	want, ok := strings.CutPrefix(a.Digest, "sha256:")
	if !ok || want == "" {
		return fmt.Errorf("download %s: release has no sha256 digest", a.Name)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL, nil)
	if err != nil {
		return err
	}
	// Bounded so a stalled connection cannot hang the update forever.
	client := &http.Client{Timeout: 15 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: HTTP %d", a.Name, resp.StatusCode)
	}

	total := resp.ContentLength
	if total <= 0 {
		total = a.Size
	}

	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	pw := &progressWriter{total: total, cb: progress}
	sum := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(f, sum), io.TeeReader(resp.Body, pw))
	closeErr := f.Close()
	if copyErr != nil {
		os.Remove(dst)
		return copyErr
	}
	if closeErr != nil {
		os.Remove(dst)
		return closeErr
	}
	if a.Size > 0 && written != a.Size {
		os.Remove(dst)
		return fmt.Errorf("download %s: size mismatch got %d want %d", a.Name, written, a.Size)
	}
	if got := hex.EncodeToString(sum.Sum(nil)); !strings.EqualFold(got, want) {
		os.Remove(dst)
		return fmt.Errorf("download %s: sha256 mismatch", a.Name)
	}
	return nil
}

// withV ensures a leading "v" so the value is canonical-form semver.
func withV(s string) string {
	if strings.HasPrefix(s, "v") {
		return s
	}
	return "v" + s
}
