//go:build linux

package procwatch

import (
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestLinuxSnapshotSeesOwnChild(t *testing.T) {
	s, err := New()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sleep", "5")
	if err := cmd.Start(); err != nil {
		t.Skip("sleep not available:", err)
	}
	defer func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() }()
	deadline := time.Now().Add(2 * time.Second)
	var snap Snapshot
	for time.Now().Before(deadline) {
		snap, err = s.Snapshot(os.Getpid(), Options{})
		if err != nil {
			t.Fatal(err)
		}
		// Right after Start the child may not have exec'd yet (empty
		// cmdline): keep sampling until it has.
		found := false
		for k, p := range snap.Procs {
			if k.PID == cmd.Process.Pid && p.Cmdline == "sleep 5" {
				found = true
				if p.PPID != os.Getpid() || p.Start == 0 {
					t.Fatalf("child: %+v", p)
				}
				if p.StartedAt.IsZero() || time.Since(p.StartedAt) > time.Minute {
					t.Fatalf("started_at: %v", p.StartedAt)
				}
			}
		}
		if found {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, self := snap.Procs[firstKey(snap, os.Getpid())]; !self {
		t.Fatal("root must be part of the snapshot")
	}
	if len(snap.Procs) < 2 {
		t.Fatalf("expected root and child, got %d", len(snap.Procs))
	}
	if snap.Truncated {
		t.Fatal("small tree must not be truncated")
	}
}

func firstKey(s Snapshot, pid int) Key {
	for k := range s.Procs {
		if k.PID == pid {
			return k
		}
	}
	return Key{}
}
