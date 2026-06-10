package autostart

import (
	"strings"
	"testing"
)

func TestPlistContent(t *testing.T) {
	out := plistContent("com.example.app", "/Applications/App.app/Contents/MacOS/app")
	for _, want := range []string{
		"com.example.app",
		"/Applications/App.app/Contents/MacOS/app",
		"<key>RunAtLoad</key>",
		"<true/>",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("plist missing %q in:\n%s", want, out)
		}
	}
}

func TestRunValue(t *testing.T) {
	got := runValue(`C:\Program Files\KUB Connect\kub-connect.exe`)
	want := `"C:\Program Files\KUB Connect\kub-connect.exe"`
	if got != want {
		t.Errorf("runValue = %q, want %q", got, want)
	}
}
