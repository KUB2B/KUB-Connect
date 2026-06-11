package updater

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

const apiURL = "https://api.github.com/repos/KUB2B/KUB-Connect/releases/latest"

type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

type Release struct {
	TagName string  `json:"tag_name"`
	HTMLURL string  `json:"html_url"`
	Assets  []Asset `json:"assets"`
}

// PickInstaller returns the Windows installer asset from a release. It matches
// the name produced by build/windows/installer/project.nsi OutFile, which ends
// in "-installer.exe" (e.g. kub-connect-amd64-installer.exe). Reports false if
// the release carries no such asset.
func PickInstaller(rel Release) (Asset, bool) {
	for _, a := range rel.Assets {
		if strings.HasSuffix(strings.ToLower(a.Name), "-installer.exe") {
			return a, true
		}
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

// withV ensures a leading "v" so the value is canonical-form semver.
func withV(s string) string {
	if strings.HasPrefix(s, "v") {
		return s
	}
	return "v" + s
}
