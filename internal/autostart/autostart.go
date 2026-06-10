// Package autostart registers the application to launch at user login. The
// concrete Manager is per-OS; obtain one with New().
package autostart

import (
	"bytes"
	"encoding/xml"
)

// Manager controls launch-on-login registration.
type Manager interface {
	// Supported reports whether autostart is implemented on this OS.
	Supported() bool
	// Enable registers the app to launch at user login, resolving its own exe
	// path. Idempotent: re-enabling overwrites the existing entry.
	Enable() error
	// Disable removes the login entry. A missing entry is not an error.
	Disable() error
}

// macLabel is the LaunchAgent label (reverse of the qb2b.pro domain); it does
// not depend on a bundle ID, which the build does not set.
const macLabel = "pro.qb2b.kub-connect"

// plistContent builds a macOS LaunchAgent plist for the given label and exe path.
// Both values are XML-escaped so a path containing &, <, or > yields valid XML.
func plistContent(label, execPath string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>` + xmlEscape(label) + `</string>
	<key>ProgramArguments</key>
	<array>
		<string>` + xmlEscape(execPath) + `</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
</dict>
</plist>
`
}

// xmlEscape returns s with XML metacharacters escaped for use in element text.
func xmlEscape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// runValue builds the Windows Run registry value: the exe path wrapped in quotes
// so paths containing spaces (e.g. Program Files) launch correctly.
func runValue(execPath string) string {
	return `"` + execPath + `"`
}
