// Package logbus is a thread-safe, capped, fan-out line buffer used to stream
// log output to the GUI. Producers call Append; the GUI subscribes for live
// lines and calls Lines once for the initial backlog.
package logbus

import "sync"

// Bus is a capped ring buffer with fan-out subscribers.
type Bus struct {
	mu     sync.Mutex
	cap    int
	lines  []string
	nextID int
	subs   map[int]func(string)
}

// New returns a Bus retaining at most capacity lines (default 1000 if <= 0).
func New(capacity int) *Bus {
	if capacity <= 0 {
		capacity = 1000
	}
	return &Bus{cap: capacity, subs: map[int]func(string){}}
}

// Append records a line and delivers it to all current subscribers.
func (b *Bus) Append(line string) {
	b.mu.Lock()
	b.lines = append(b.lines, line)
	if len(b.lines) > b.cap {
		b.lines = b.lines[len(b.lines)-b.cap:]
	}
	subs := make([]func(string), 0, len(b.subs))
	for _, fn := range b.subs {
		subs = append(subs, fn)
	}
	b.mu.Unlock()
	for _, fn := range subs {
		fn(line)
	}
}

// Lines returns a copy of the retained lines.
func (b *Bus) Lines() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]string, len(b.lines))
	copy(out, b.lines)
	return out
}

// Subscribe registers fn to receive every subsequent Append. The returned
// function unsubscribes.
func (b *Bus) Subscribe(fn func(string)) (cancel func()) {
	b.mu.Lock()
	id := b.nextID
	b.nextID++
	b.subs[id] = fn
	b.mu.Unlock()
	return func() {
		b.mu.Lock()
		delete(b.subs, id)
		b.mu.Unlock()
	}
}
