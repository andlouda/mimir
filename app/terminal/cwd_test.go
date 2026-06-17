package terminal

import "testing"

func TestLastReportedCwd(t *testing.T) {
	m := NewManager()

	if got := m.GetLastReportedCwd(1); got != "" {
		t.Errorf("initial cwd = %q, want empty", got)
	}

	m.setLastReportedCwd(1, "/home/u/proj")
	if got := m.GetLastReportedCwd(1); got != "/home/u/proj" {
		t.Errorf("cwd = %q, want /home/u/proj", got)
	}

	// An empty report must not clobber a known directory.
	m.setLastReportedCwd(1, "")
	if got := m.GetLastReportedCwd(1); got != "/home/u/proj" {
		t.Errorf("cwd after empty report = %q, want /home/u/proj", got)
	}

	// Sessions are tracked independently.
	if got := m.GetLastReportedCwd(2); got != "" {
		t.Errorf("unset session cwd = %q, want empty", got)
	}
}
