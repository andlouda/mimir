package timeline

import (
	"sync"
	"time"
)

// Batcher groups events per terminal and hands them on at most once per
// window (or when a batch is full), so a burst of hook and telemetry
// events becomes a few UI updates instead of one per record. Events
// without a terminal are dropped: nobody is looking at them live.
type Batcher struct {
	window time.Duration
	max    int
	emit   func(terminalID int, events []Event)

	mu      sync.Mutex
	pending map[int][]Event
	timers  map[int]*time.Timer
	closed  bool
}

// NewBatcher creates a batcher; emit runs on a timer goroutine.
func NewBatcher(window time.Duration, max int, emit func(terminalID int, events []Event)) *Batcher {
	if window <= 0 {
		window = 250 * time.Millisecond
	}
	if max <= 0 {
		max = 200
	}
	return &Batcher{window: window, max: max, emit: emit, pending: map[int][]Event{}, timers: map[int]*time.Timer{}}
}

// Add queues an event for its terminal.
func (b *Batcher) Add(ev Event) {
	if ev.TerminalID <= 0 {
		return
	}
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.pending[ev.TerminalID] = append(b.pending[ev.TerminalID], ev)
	full := len(b.pending[ev.TerminalID]) >= b.max
	if full {
		if t := b.timers[ev.TerminalID]; t != nil {
			t.Stop()
			delete(b.timers, ev.TerminalID)
		}
		b.mu.Unlock()
		b.Flush(ev.TerminalID)
		return
	}
	if _, armed := b.timers[ev.TerminalID]; !armed {
		id := ev.TerminalID
		b.timers[id] = time.AfterFunc(b.window, func() { b.Flush(id) })
	}
	b.mu.Unlock()
}

// Flush emits what is pending for a terminal.
func (b *Batcher) Flush(terminalID int) {
	b.mu.Lock()
	events := b.pending[terminalID]
	delete(b.pending, terminalID)
	delete(b.timers, terminalID)
	b.mu.Unlock()
	if len(events) > 0 && b.emit != nil {
		b.emit(terminalID, events)
	}
}

// Close stops the timers and drops what is pending.
func (b *Batcher) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	for id, t := range b.timers {
		t.Stop()
		delete(b.timers, id)
	}
	b.pending = map[int][]Event{}
}
