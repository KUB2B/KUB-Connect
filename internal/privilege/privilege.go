// Package privilege reports whether the process has the OS privileges
// (root / Administrator) required for TUN-mode networking.
package privilege

import "fmt"

// RequireElevated returns a descriptive error if the process is not elevated.
// purpose names the feature needing elevation, for the message.
func RequireElevated(purpose string) error {
	if IsElevated() {
		return nil
	}
	return fmt.Errorf("%s requires administrator/root privileges; re-run elevated or use proxy mode", purpose)
}
