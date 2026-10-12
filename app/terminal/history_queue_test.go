package terminal

import (
	"sync"
	"testing"
	"time"

	"mimir/history"
)

type fakeInserter struct {
	mu      sync.Mutex
	entries []history.CommandEntry
	// block, when set, parks Insert until closed; entered reports that the
	// worker has reached Insert (so a test knows the worker is busy).
	block   chan struct{}
	entered chan struct{}
}

func (f *fakeInserter) Insert(e history.CommandEntry) error {
	if f.entered != nil {
		select {
		case f.entered <- struct{}{}:
		default:
		}
	}
	if f.block != nil {
		<-f.block
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = append(f.entries, e)
	return nil
}

func (f *fakeInserter) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.entries)
}

func TestHistoryWorkerWritesQueuedEntriesAndDrainsOnStop(t *testing.T) {
	m := NewManager()
	store := &fakeInserter{}
	m.ptyMutex.Lock()
	m.startHistoryWorker(store)
	m.startHistoryWorker(store) // idempotent
	m.ptyMutex.Unlock()

	for i := 0; i < 10; i++ {
		m.enqueueHistory(history.CommandEntry{SessionID: 1, Command: "ls"})
	}
	m.stopHistoryWorker()
	if got := store.count(); got != 10 {
		t.Fatalf("expected 10 inserts after drain, got %d", got)
	}
	// After stop the queue is gone: enqueue is a no-op, no panic.
	m.enqueueHistory(history.CommandEntry{Command: "late"})
	m.stopHistoryWorker()
	if got := store.count(); got != 10 {
		t.Fatalf("entry after stop must be dropped, got %d", got)
	}
}

func TestHistoryWorkerDropsWhenQueueIsFull(t *testing.T) {
	m := NewManager()
	store := &fakeInserter{block: make(chan struct{}), entered: make(chan struct{}, 1)}
	m.ptyMutex.Lock()
	m.startHistoryWorker(store)
	m.ptyMutex.Unlock()

	// Park the worker on the first entry before flooding, otherwise the
	// count depends on whether it had dequeued yet (seen on macOS CI).
	m.enqueueHistory(history.CommandEntry{Command: "x"})
	select {
	case <-store.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not pick up the first entry")
	}
	// The queue holds the next historyQueueSize; everything beyond is
	// dropped without blocking.
	start := time.Now()
	for i := 0; i < historyQueueSize+50; i++ {
		m.enqueueHistory(history.CommandEntry{Command: "x"})
	}
	if time.Since(start) > time.Second {
		t.Fatalf("enqueue must never block, took %s", time.Since(start))
	}
	close(store.block)
	m.stopHistoryWorker()
	if got := store.count(); got != historyQueueSize+1 {
		t.Fatalf("expected %d inserts (queue + the one in flight), got %d", historyQueueSize+1, got)
	}
}
