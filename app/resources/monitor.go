// Package resources samples the process table and reports, per pane, what
// the process tree below the pane's shell consumes. CPU is derived from
// cumulative CPU time between two samples, so the first call reports 0.
// Nothing is signalled or limited here; this is a read-only view.
package resources

import (
	"sync"
	"time"
)

// Sample is one process as seen in the process table.
type Sample struct {
	PID        int
	PPID       int
	RSS        int64   // resident set, bytes
	CPUSeconds float64 // cumulative user+system time
}

// Usage is what a pane's process tree consumes.
type Usage struct {
	// CPU is the share of one core in percent over the last interval
	// (200 means two cores busy).
	CPU float64 `json:"cpu"`
	// RSS sums the resident sets of the tree, bytes (shared pages are
	// counted per process, so this is an upper bound).
	RSS int64 `json:"rss"`
	// Procs is the number of processes in the tree, the shell included.
	Procs int `json:"procs"`
}

// Monitor keeps the previous sample so CPU can be computed as a delta.
type Monitor struct {
	mu      sync.Mutex
	prev    map[int]float64
	prevAt  time.Time
	sampler func() ([]Sample, error)
	now     func() time.Time
}

// NewMonitor returns a monitor backed by the platform process table.
func NewMonitor() *Monitor {
	return &Monitor{sampler: sampleProcesses, now: time.Now}
}

// Usage returns the usage of every tree rooted at roots (key → root pid).
// Roots that no longer exist are left out.
func (m *Monitor) Usage(roots map[string]int) (map[string]Usage, error) {
	samples, err := m.sampler()
	if err != nil {
		return nil, err
	}
	now := m.now()
	byPID := make(map[int]Sample, len(samples))
	children := make(map[int][]int, len(samples))
	for _, s := range samples {
		byPID[s.PID] = s
		children[s.PPID] = append(children[s.PPID], s.PID)
	}

	m.mu.Lock()
	prev, prevAt := m.prev, m.prevAt
	next := make(map[int]float64, len(samples))
	for _, s := range samples {
		next[s.PID] = s.CPUSeconds
	}
	m.prev, m.prevAt = next, now
	m.mu.Unlock()

	elapsed := now.Sub(prevAt).Seconds()
	out := make(map[string]Usage, len(roots))
	for key, root := range roots {
		if _, ok := byPID[root]; !ok {
			continue
		}
		var u Usage
		var cpuDelta float64
		stack := []int{root}
		seen := map[int]bool{}
		for len(stack) > 0 {
			pid := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if seen[pid] {
				continue
			}
			seen[pid] = true
			s := byPID[pid]
			u.Procs++
			u.RSS += s.RSS
			if before, ok := prev[pid]; ok && s.CPUSeconds >= before {
				cpuDelta += s.CPUSeconds - before
			}
			stack = append(stack, children[pid]...)
		}
		if prev != nil && elapsed > 0.2 {
			u.CPU = cpuDelta / elapsed * 100
			if u.CPU < 0 {
				u.CPU = 0
			}
		}
		out[key] = u
	}
	return out, nil
}
