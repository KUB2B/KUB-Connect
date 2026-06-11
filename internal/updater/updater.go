package updater

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

const apiURL = "https://api.github.com/repos/KUB2B/KUB-Connect/releases/latest"

type Release struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
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

// IsNewer reports whether latest tag is a different version than current.
// Returns false for dev builds so CI/dev users don't see spurious banners.
func IsNewer(current, latestTag string) bool {
	if current == "dev" || current == "" || latestTag == "" {
		return false
	}
	cur := strings.TrimPrefix(current, "v")
	lat := strings.TrimPrefix(latestTag, "v")
	return lat != cur
}
