package logbus

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// waitForLine polls until bus contains want or the deadline passes.
func waitForLine(t *testing.T, bus *Bus, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, l := range bus.Lines() {
			if l == want {
				return
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for line %q; got %v", want, bus.Lines())
}

func appendLine(t *testing.T, path, line string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("open for append: %v", err)
	}
	defer f.Close()
	if _, err := f.WriteString(line + "\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestTailFileStreamsAppendedLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "xray.log")
	if err := os.WriteFile(path, []byte("first\n"), 0o600); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	bus := New(100)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go TailFile(ctx, path, bus, 10*time.Millisecond)

	waitForLine(t, bus, "first")
	appendLine(t, path, "second")
	waitForLine(t, bus, "second")
}

func TestTailFileMissingFileDoesNotPanic(t *testing.T) {
	bus := New(10)
	ctx, cancel := context.WithCancel(context.Background())
	go TailFile(ctx, filepath.Join(t.TempDir(), "nope.log"), bus, 5*time.Millisecond)
	time.Sleep(30 * time.Millisecond) // a few poll cycles against a missing file
	cancel()
	if len(bus.Lines()) != 0 {
		t.Errorf("expected no lines from missing file, got %v", bus.Lines())
	}
}
