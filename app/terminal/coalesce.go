package terminal

import (
	"sync"
	"time"
)

// outputCoalescer joins PTY output into one event per short interval.
// A TUI that redraws its screen (Claude Code's spinner, progress views)
// produces dozens of 4 KB reads per second per pane; emitting each as its
// own Wails event meant JSON-encoding, IPC and an xterm parse+render per
// read. Batching at ~12 ms keeps latency below a frame and cuts the event
// count by an order of magnitude under load. Large bursts flush at once.

const (
	coalesceWindow = 12 * time.Millisecond
	coalesceMax    = 128 * 1024
)

type outputCoalescer struct {
	mu     sync.Mutex
	buf    []byte
	timer  *time.Timer
	emit   func([]byte)
	closed bool
}

func newOutputCoalescer(emit func([]byte)) *outputCoalescer {
	return &outputCoalescer{emit: emit}
}

// Write queues data; it is emitted with whatever arrives within the window.
func (c *outputCoalescer) Write(data []byte) {
	if len(data) == 0 {
		return
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.buf = append(c.buf, data...)
	if len(c.buf) >= coalesceMax {
		out := c.buf
		c.buf = nil
		if c.timer != nil {
			c.timer.Stop()
			c.timer = nil
		}
		c.mu.Unlock()
		c.emit(out)
		return
	}
	if c.timer == nil {
		c.timer = time.AfterFunc(coalesceWindow, c.flush)
	}
	c.mu.Unlock()
}

func (c *outputCoalescer) flush() {
	c.mu.Lock()
	out := c.buf
	c.buf = nil
	c.timer = nil
	c.mu.Unlock()
	if len(out) > 0 {
		c.emit(out)
	}
}

// Close emits what is pending and drops everything written afterwards.
func (c *outputCoalescer) Close() {
	c.mu.Lock()
	c.closed = true
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
	out := c.buf
	c.buf = nil
	c.mu.Unlock()
	if len(out) > 0 {
		c.emit(out)
	}
}
