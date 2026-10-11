package procwatch

import (
	"testing"
	"time"
)

func TestParseStatAndCmdline(t *testing.T) {
	line := "1234 (tmux: server) S 1 1234 1234 0 -1 4194560 2191 0 0 0 7 3 0 0 20 0 1 0 987654 12345678 500 18446744073709551615 1 1 0 0 0 0 0 0 0 0 0 0 17 3 0 0 0 0 0"
	ppid, start, ok := parseStat(line)
	if !ok || ppid != 1 || start != 987654 {
		t.Fatalf("stat: %d %d %v", ppid, start, ok)
	}
	if _, _, ok := parseStat("garbage"); ok {
		t.Fatal("garbage must fail")
	}
	if got := parseCmdline([]byte("python\x00analyze.py\x00--fast\x00"), 0); got != "python analyze.py --fast" {
		t.Fatalf("cmdline: %q", got)
	}
	if got := parseCmdline([]byte("abcdefghij"), 4); got != "abcd...[truncated]" {
		t.Fatalf("cap: %q", got)
	}
	if n, ok := parseSocketInode("socket:[31337]"); !ok || n != 31337 {
		t.Fatal("socket inode")
	}
	if _, ok := parseSocketInode("/dev/null"); ok {
		t.Fatal("non-socket must not parse")
	}
	if kids := parseChildren("12 34  56 \n"); len(kids) != 3 || kids[2] != 56 {
		t.Fatalf("children: %v", kids)
	}
}

func TestParseNetTableAndAddresses(t *testing.T) {
	tcp := "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n" +
		"   0: 0100007F:0BB8 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 31337 1 0000000000000000 100 0 0 10 0\n" +
		"   1: 0F02000A:D2A4 097100CB:01BB 01 00000000:00000000 00:00000000 00000000  1000        0 31338 1 0000000000000000 20 4 30 10 -1\n" +
		"   2: 0100007F:0001 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 0 1 0000000000000000 100 0 0 10 0\n"
	entries := parseNetTable(tcp, "tcp")
	if len(entries) != 2 {
		t.Fatalf("entries: %+v", entries)
	}
	if entries[0].Inode != 31337 || entries[0].Local != "127.0.0.1:3000" || entries[0].State != "LISTEN" {
		t.Fatalf("listen row: %+v", entries[0])
	}
	if entries[1].Local != "10.0.2.15:53924" || entries[1].Remote != "203.0.113.9:443" || entries[1].State != "ESTABLISHED" {
		t.Fatalf("established row: %+v", entries[1])
	}
	tcp6 := "  sl  local_address                         remote_address                        st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n" +
		"   0: 00000000000000000000000001000000:1F90 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 4242 1 0000000000000000 100 0 0 10 0\n" +
		"   1: 0000000000000000FFFF00000100007F:C350 0000000000000000FFFF0000097100CB:01BB 01 00000000:00000000 00:00000000 00000000  1000        0 4243 1 0000000000000000 20 4 30 10 -1\n"
	e6 := parseNetTable(tcp6, "tcp6")
	if len(e6) != 2 || e6[0].Local != "[::1]:8080" || e6[1].Remote != "203.0.113.9:443" || e6[1].Local != "127.0.0.1:50000" {
		t.Fatalf("v6 rows: %+v", e6)
	}
	udp := "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode ref pointer drops\n" +
		"  1: 0100007F:0035 00000000:0000 07 00000000:00000000 00:00000000 00000000  1000        0 555 2 0000000000000000 0\n"
	u := parseNetTable(udp, "udp")
	if len(u) != 1 || u[0].State != "" || u[0].Local != "127.0.0.1:53" {
		t.Fatalf("udp: %+v", u)
	}
	if hexAddr("nonsense") != "nonsense" {
		t.Fatal("unparseable address must pass through")
	}
}

func TestDiffReportsStartsEndsOpensCloses(t *testing.T) {
	at1 := time.Unix(100, 0)
	at2 := time.Unix(101, 0)
	a := Snapshot{At: at1, Procs: map[Key]Proc{
		{PID: 1, Start: 10}: {PID: 1, Start: 10, Cmdline: "claude"},
		{PID: 2, Start: 20}: {PID: 2, PPID: 1, Start: 20, Cmdline: "bash -c sleep"},
	}, Conns: map[uint64]Conn{7: {Inode: 7, PID: 2, Remote: "203.0.113.9:443"}}}
	first := Diff(nil, a)
	if len(first) != 3 || first[0].Kind != ProcessStart || first[0].Proc.PID != 1 || first[1].Proc.PID != 2 || first[2].Kind != ConnOpen {
		t.Fatalf("initial diff: %+v", first)
	}
	b := Snapshot{At: at2, Procs: map[Key]Proc{
		{PID: 1, Start: 10}: {PID: 1, Start: 10, Cmdline: "claude"},
		{PID: 2, Start: 30}: {PID: 2, PPID: 1, Start: 30, Cmdline: "curl https://x"}, // pid reused
	}, Conns: map[uint64]Conn{8: {Inode: 8, PID: 2, Remote: "198.51.100.4:443"}}}
	changes := Diff(&a, b)
	kinds := []string{}
	for _, c := range changes {
		kinds = append(kinds, c.Kind)
	}
	want := []string{ProcessEnd, ConnClose, ProcessStart, ConnOpen}
	if len(kinds) != 4 || kinds[0] != want[0] || kinds[1] != want[1] || kinds[2] != want[2] || kinds[3] != want[3] {
		t.Fatalf("kinds: %v", kinds)
	}
	if changes[0].Proc.Cmdline != "bash -c sleep" || changes[2].Proc.Cmdline != "curl https://x" || !changes[2].At.Equal(at2) {
		t.Fatalf("pid reuse must be an end and a start: %+v", changes)
	}
	if len(Diff(&b, b)) != 0 {
		t.Fatal("identical snapshots must not differ")
	}
}
