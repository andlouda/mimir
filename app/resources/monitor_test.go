package resources

import (
	"testing"
	"time"
)

func TestUsageSumsTreesAndDerivesCPUFromDeltas(t *testing.T) {
	// 1 → 10 (shell) → 11 (agent) → 12 (test runner); 20 is another pane.
	first := []Sample{
		{PID: 1, PPID: 0, RSS: 1, CPUSeconds: 0},
		{PID: 10, PPID: 1, RSS: 100, CPUSeconds: 1.0},
		{PID: 11, PPID: 10, RSS: 200, CPUSeconds: 5.0},
		{PID: 12, PPID: 11, RSS: 300, CPUSeconds: 2.0},
		{PID: 20, PPID: 1, RSS: 50, CPUSeconds: 0.5},
	}
	second := []Sample{
		{PID: 1, PPID: 0, RSS: 1, CPUSeconds: 0},
		{PID: 10, PPID: 1, RSS: 100, CPUSeconds: 1.0},
		{PID: 11, PPID: 10, RSS: 250, CPUSeconds: 7.0}, // +2 s
		{PID: 12, PPID: 11, RSS: 300, CPUSeconds: 3.0}, // +1 s
		{PID: 20, PPID: 1, RSS: 50, CPUSeconds: 0.5},
	}
	at := time.Unix(1000, 0)
	cur := first
	m := &Monitor{sampler: func() ([]Sample, error) { return cur, nil }, now: func() time.Time { return at }}
	u, err := m.Usage(map[string]int{"a": 10, "b": 20, "gone": 99})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := u["gone"]; ok {
		t.Fatal("missing root must be left out")
	}
	if u["a"].Procs != 3 || u["a"].RSS != 600 || u["a"].CPU != 0 {
		t.Fatalf("first sample: %+v", u["a"])
	}
	cur, at = second, at.Add(2*time.Second)
	u, _ = m.Usage(map[string]int{"a": 10, "b": 20})
	if u["a"].CPU != 150 || u["a"].RSS != 650 || u["a"].Procs != 3 {
		t.Fatalf("second sample: %+v (want 3 s of CPU over 2 s = 150 %%)", u["a"])
	}
	if u["b"].CPU != 0 || u["b"].Procs != 1 {
		t.Fatalf("idle pane: %+v", u["b"])
	}
}

func TestParseProcStat(t *testing.T) {
	line := "4242 (node (x)) S 4000 4242 4000 0 -1 4194560 100 0 0 0 150 50 0 0 20 0 7 0 12345 123456789 2500 18446744073709551615 1 1 0 0 0 0 0 0 0 0 0 0 17 3 0 0 0 0 0"
	s, ok := parseProcStat(line, 4096, 100)
	if !ok || s.PID != 4242 || s.PPID != 4000 || s.CPUSeconds != 2.0 || s.RSS != 2500*4096 {
		t.Fatalf("got %+v ok=%v", s, ok)
	}
}

func TestParsePSAndTime(t *testing.T) {
	out := "  1     0  1200 0:01.50\n 300     1 20480 1-02:03:04\nbad line\n"
	s := parsePS(out)
	if len(s) != 2 || s[0].CPUSeconds != 1.5 || s[0].RSS != 1200*1024 || s[1].CPUSeconds != 86400+2*3600+3*60+4 {
		t.Fatalf("got %+v", s)
	}
	w := parseTSV("4\t0\t8192\t15000000\r\n")
	if len(w) != 1 || w[0].RSS != 8192 || w[0].CPUSeconds != 1.5 {
		t.Fatalf("tsv: %+v", w)
	}
}
