package timeline

import (
	"sync"
	"testing"
	"time"
)

func TestBatcherGroupsPerTerminalAndFlushesWhenFull(t *testing.T) {
	var mu sync.Mutex
	got := map[int][][]Event{}
	b := NewBatcher(40*time.Millisecond, 3, func(tid int, evs []Event) {
		mu.Lock()
		got[tid] = append(got[tid], evs)
		mu.Unlock()
	})
	defer b.Close()
	for i := 0; i < 3; i++ {
		b.Add(Event{TerminalID: 1, Seq: uint64(i + 1)})
	}
	b.Add(Event{TerminalID: 2, Seq: 1})
	b.Add(Event{TerminalID: 0, Seq: 9}) // unbound: dropped
	mu.Lock()
	if len(got[1]) != 1 || len(got[1][0]) != 3 {
		t.Fatalf("full batch must flush at once: %+v", got[1])
	}
	if len(got[2]) != 0 {
		t.Fatal("terminal 2 must wait for the window")
	}
	mu.Unlock()
	time.Sleep(120 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(got[2]) != 1 || got[2][0][0].Seq != 1 {
		t.Fatalf("window flush: %+v", got[2])
	}
	if _, ok := got[0]; ok {
		t.Fatal("unbound events must never be emitted")
	}
}
