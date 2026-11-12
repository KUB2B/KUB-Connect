//go:build !windows

package sysproxy

// ClearStale is a no-op outside Windows.
func ClearStale() (bool, error) { return false, nil }
