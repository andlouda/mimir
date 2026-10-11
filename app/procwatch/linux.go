//go:build linux

package procwatch

import (
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultMaxProcs = 300
	defaultMaxFDs   = 2000
	cmdlineCap      = 4096
	clkTck          = 100 // USER_HZ, what /proc reports regardless of the kernel HZ
)

type linuxSampler struct {
	btime int64 // boot time (unix seconds) for StartedAt; 0 when unknown
}

// New returns the /proc sampler.
func New() (Sampler, error) {
	return &linuxSampler{btime: readBtime()}, nil
}

func readBtime() int64 {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "btime ") {
			n, _ := strconv.ParseInt(strings.TrimSpace(line[len("btime "):]), 10, 64)
			return n
		}
	}
	return 0
}

// Snapshot walks the tree below root through /proc/<pid>/task/<pid>/children
// (falls back to a full /proc scan when the kernel lacks that file), reads
// each process's stat and cmdline, collects socket inodes from its fds and
// resolves them against /proc/net once per snapshot.
func (s *linuxSampler) Snapshot(root int, opts Options) (Snapshot, error) {
	maxProcs := opts.MaxProcs
	if maxProcs <= 0 {
		maxProcs = defaultMaxProcs
	}
	maxFDs := opts.MaxFDs
	if maxFDs <= 0 {
		maxFDs = defaultMaxFDs
	}
	snap := Snapshot{At: time.Now(), Procs: map[Key]Proc{}, Conns: map[uint64]Conn{}}
	if root <= 0 {
		return snap, nil
	}
	pids := s.subtree(root, maxProcs, &snap.Truncated)
	inodeOwner := map[uint64]int{}
	fds := 0
	for _, pid := range pids {
		dir := "/proc/" + strconv.Itoa(pid)
		stat, err := os.ReadFile(dir + "/stat")
		if err != nil {
			continue // exited meanwhile
		}
		ppid, start, ok := parseStat(string(stat))
		if !ok {
			continue
		}
		cmd, _ := os.ReadFile(dir + "/cmdline")
		p := Proc{PID: pid, PPID: ppid, Start: start, Cmdline: parseCmdline(cmd, cmdlineCap)}
		if s.btime > 0 {
			p.StartedAt = time.Unix(s.btime, 0).Add(time.Duration(start) * time.Second / clkTck)
		}
		snap.Procs[Key{PID: pid, Start: start}] = p
		if fds >= maxFDs {
			snap.Truncated = true
			continue
		}
		entries, err := os.ReadDir(dir + "/fd")
		if err != nil {
			continue // another user's process, or gone
		}
		for _, e := range entries {
			fds++
			if fds > maxFDs {
				snap.Truncated = true
				break
			}
			link, err := os.Readlink(dir + "/fd/" + e.Name())
			if err != nil {
				continue
			}
			if inode, ok := parseSocketInode(link); ok {
				inodeOwner[inode] = pid
			}
		}
	}
	if len(inodeOwner) == 0 {
		return snap, nil
	}
	tables := []string{"tcp", "tcp6"}
	if opts.UDP {
		tables = append(tables, "udp", "udp6")
	}
	for _, proto := range tables {
		data, err := os.ReadFile("/proc/net/" + proto)
		if err != nil {
			continue
		}
		for _, e := range parseNetTable(string(data), proto) {
			pid, ok := inodeOwner[e.Inode]
			if !ok {
				continue
			}
			snap.Conns[e.Inode] = Conn{Inode: e.Inode, Proto: proto, Local: e.Local, Remote: e.Remote, State: e.State, PID: pid}
		}
	}
	return snap, nil
}

// subtree lists root and its descendants, breadth first, capped.
func (s *linuxSampler) subtree(root, max int, truncated *bool) []int {
	out := []int{root}
	queue := []int{root}
	seen := map[int]bool{root: true}
	childrenFile := true
	for len(queue) > 0 && len(out) < max {
		pid := queue[0]
		queue = queue[1:]
		var kids []int
		if childrenFile {
			data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/task/" + strconv.Itoa(pid) + "/children")
			if err != nil && os.IsNotExist(err) && pid == root {
				childrenFile = false // kernel without CONFIG_PROC_CHILDREN
			} else {
				kids = parseChildren(string(data))
			}
		}
		if !childrenFile {
			kids = scanChildren(pid)
		}
		for _, k := range kids {
			if seen[k] {
				continue
			}
			seen[k] = true
			if len(out) >= max {
				*truncated = true
				return out
			}
			out = append(out, k)
			queue = append(queue, k)
		}
	}
	if len(queue) > 0 {
		*truncated = true
	}
	return out
}

// scanChildren is the fallback: every /proc/<pid>/stat whose ppid matches.
func scanChildren(parent int) []int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	var out []int
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		stat, err := os.ReadFile("/proc/" + e.Name() + "/stat")
		if err != nil {
			continue
		}
		if ppid, _, ok := parseStat(string(stat)); ok && ppid == parent {
			out = append(out, pid)
		}
	}
	return out
}
