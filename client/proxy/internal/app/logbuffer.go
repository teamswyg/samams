package app

import "sync"

// LogBuffer is a simple in-memory ring buffer of log lines.
type LogBuffer struct {
	mu    sync.Mutex
	lines []string
	max   int
}

func NewLogBuffer(max int) *LogBuffer {
	if max <= 0 {
		max = 200
	}
	return &LogBuffer{
		lines: make([]string, 0, max),
		max:   max,
	}
}

func (b *LogBuffer) Append(line string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.lines) < b.max {
		b.lines = append(b.lines, line)
		return
	}

	copy(b.lines, b.lines[1:])
	b.lines[len(b.lines)-1] = line
}

func (b *LogBuffer) Lines() []string {
	b.mu.Lock()
	defer b.mu.Unlock()

	out := make([]string, len(b.lines))
	copy(out, b.lines)
	return out
}

func (b *LogBuffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lines = b.lines[:0]
}
