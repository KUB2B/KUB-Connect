package updater

import "testing"

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
