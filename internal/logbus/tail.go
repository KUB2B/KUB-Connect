package logbus

import (
	"bufio"
	"context"
	"io"
	"os"
	"time"
)

// TailFile polls path every interval, appending any newly written lines to
// bus, until ctx is cancelled. A missing or unreadable file is skipped (the
// xray log may not exist until the first connection). The current read offset
// is tracked across polls; a truncated file is handled by resetting to start.
func TailFile(ctx context.Context, path string, bus *Bus, interval time.Duration) {
	var offset int64
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		offset = readFrom(path, offset, bus)
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}
	}
}

// readFrom opens path, reads complete lines from offset to EOF into bus, and
// returns the new offset. On any error it returns offset unchanged.
func readFrom(path string, offset int64, bus *Bus) int64 {
	f, err := os.Open(path)
	if err != nil {
		return offset
	}
	defer f.Close()

	size, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return offset
	}
	if offset > size { // file was truncated/rotated; start over
		offset = 0
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return offset
	}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		bus.Append(sc.Text())
	}
	pos, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		return offset
	}
	return pos
}
