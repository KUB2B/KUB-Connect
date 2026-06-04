package wintundll

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractWritesFile(t *testing.T) {
	dst := t.TempDir()
	path, err := extract([]byte("dll-bytes"), dst, "wintun.dll")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if path != filepath.Join(dst, "wintun.dll") {
		t.Errorf("path = %q, want %q", path, filepath.Join(dst, "wintun.dll"))
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "dll-bytes" {
		t.Errorf("contents = %q, want %q", got, "dll-bytes")
	}
}

func TestExtractSkipsWhenSizeMatches(t *testing.T) {
	dst := t.TempDir()
	existing := filepath.Join(dst, "wintun.dll")
	if err := os.WriteFile(existing, []byte("XXXXX"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Same size (5 bytes), different content: extract must not rewrite.
	if _, err := extract([]byte("12345"), dst, "wintun.dll"); err != nil {
		t.Fatalf("extract: %v", err)
	}
	got, err := os.ReadFile(existing)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "XXXXX" {
		t.Errorf("rewritten: got %q, want untouched %q", got, "XXXXX")
	}
}

func TestExtractRewritesWhenSizeDiffers(t *testing.T) {
	dst := t.TempDir()
	stale := filepath.Join(dst, "wintun.dll")
	if err := os.WriteFile(stale, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := extract([]byte("new-longer-bytes"), dst, "wintun.dll"); err != nil {
		t.Fatalf("extract: %v", err)
	}
	got, err := os.ReadFile(stale)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new-longer-bytes" {
		t.Errorf("not rewritten: got %q", got)
	}
}
