package terminal

import (
	"bytes"
	"sync"
	"testing"
	"time"
)

func TestOutputCoalescerJoinsWritesWithinTheWindow(t *testing.T) {
	var mu sync.Mutex
	var events [][]byte
	c := newOutputCoalescer(func(b []byte) { mu.Lock(); events = append(events, append([]byte(nil), b...)); mu.Unlock() })
	for i := 0; i < 20; i++ {
		c.Write([]byte("x"))
	}
	time.Sleep(coalesceWindow * 4)
	mu.Lock()
	n, first := len(events), events[0]
	mu.Unlock()
	if n != 1 || !bytes.Equal(first, bytes.Repeat([]byte("x"), 20)) {
		t.Fatalf("20 writes within the window must become one event, got %d: %q", n, first)
	}
	// A burst above the cap goes out immediately, order preserved.
	big := bytes.Repeat([]byte("y"), coalesceMax)
	c.Write([]byte("a"))
	c.Write(big)
	mu.Lock()
	n = len(events)
	mu.Unlock()
	if n != 2 {
		t.Fatalf("burst above the cap must flush at once, got %d events", n)
	}
	mu.Lock()
	second := events[1]
	mu.Unlock()
	if second[0] != 'a' || len(second) != coalesceMax+1 {
		t.Fatalf("burst must keep order and content, got len %d", len(second))
	}
	// Close flushes the rest and ignores later writes.
	c.Write([]byte("tail"))
	c.Close()
	c.Write([]byte("late"))
	time.Sleep(coalesceWindow * 2)
	mu.Lock()
	defer mu.Unlock()
	if len(events) != 3 || string(events[2]) != "tail" {
		t.Fatalf("close must flush pending and drop later writes, got %d events, last %q", len(events), events[len(events)-1])
	}
}
