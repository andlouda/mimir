// Package procwatch takes cheap snapshots of the process tree below a pid
// and of the TCP/UDP sockets those processes hold, and turns consecutive
// snapshots into start/end and open/close changes. It is the runtime side
// of the agent timeline: what really ran while the agent called a tool.
//
// Linux reads /proc without privileges (own processes only); other
// platforms report ErrUnsupported until they get a sampler.
package procwatch

import (
	"errors"
	"sort"
	"time"
)

// ErrUnsupported is returned by New on platforms without a sampler.
var ErrUnsupported = errors.New("process observation is not supported on this platform")

// Options bound one snapshot.
type Options struct {
	// UDP includes UDP sockets (mostly DNS noise; off by default).
	UDP bool
	// MaxProcs caps the processes walked (0 = 300); MaxFDs caps the file
	// descriptors inspected per snapshot (0 = 2000). Beyond either the
	// snapshot is marked Truncated.
	MaxProcs int
	MaxFDs   int
}

// Key identifies a process across pid reuse: the pid plus its start time
// in clock ticks since boot.
type Key struct {
	PID   int
	Start uint64
}

// Proc is one process of the tree.
type Proc struct {
	PID     int
	PPID    int
	Start   uint64 // clock ticks since boot
	Cmdline string
	// StartedAt is the wall-clock start when the platform can tell.
	StartedAt time.Time
}

// Conn is one socket owned by a process of the tree.
type Conn struct {
	Inode  uint64
	Proto  string // tcp | tcp6 | udp | udp6
	Local  string
	Remote string
	State  string // ESTABLISHED, LISTEN, ... ("" for udp)
	PID    int
}

// Snapshot is the tree at one instant.
type Snapshot struct {
	At        time.Time
	Procs     map[Key]Proc
	Conns     map[uint64]Conn
	Truncated bool
}

// Change kinds.
const (
	ProcessStart = "process_start"
	ProcessEnd   = "process_end"
	ConnOpen     = "conn_open"
	ConnClose    = "conn_close"
)

// Change is one difference between two snapshots.
type Change struct {
	Kind string
	At   time.Time
	Proc Proc
	Conn Conn
}

// Sampler takes snapshots of the tree below a root pid.
type Sampler interface {
	Snapshot(root int, opts Options) (Snapshot, error)
}

// Diff lists what appeared and disappeared between prev and next. A nil
// prev reports every process and connection of next as new. Changes are
// ordered: ends and closes first, then starts (parents before children),
// then opens.
func Diff(prev *Snapshot, next Snapshot) []Change {
	var out []Change
	if prev != nil {
		for k, p := range prev.Procs {
			if _, ok := next.Procs[k]; !ok {
				out = append(out, Change{Kind: ProcessEnd, At: next.At, Proc: p})
			}
		}
		for inode, c := range prev.Conns {
			if _, ok := next.Conns[inode]; !ok {
				out = append(out, Change{Kind: ConnClose, At: next.At, Conn: c})
			}
		}
	}
	var starts []Proc
	for k, p := range next.Procs {
		if prev == nil {
			starts = append(starts, p)
			continue
		}
		if _, ok := prev.Procs[k]; !ok {
			starts = append(starts, p)
		}
	}
	sort.Slice(starts, func(i, j int) bool {
		if starts[i].Start != starts[j].Start {
			return starts[i].Start < starts[j].Start
		}
		return starts[i].PID < starts[j].PID
	})
	for _, p := range starts {
		out = append(out, Change{Kind: ProcessStart, At: next.At, Proc: p})
	}
	var opens []Conn
	for inode, c := range next.Conns {
		if prev == nil {
			opens = append(opens, c)
			continue
		}
		if _, ok := prev.Conns[inode]; !ok {
			opens = append(opens, c)
		}
	}
	sort.Slice(opens, func(i, j int) bool { return opens[i].Inode < opens[j].Inode })
	for _, c := range opens {
		out = append(out, Change{Kind: ConnOpen, At: next.At, Conn: c})
	}
	return out
}
