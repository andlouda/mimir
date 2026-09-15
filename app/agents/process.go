package agents

import (
	"bufio"
	"path"
	"strconv"
	"strings"
)

// Process is one row of a process listing.
type Process struct {
	PID  int
	PPID int
	Args string
}

// Detection is the result of scanning a terminal's process tree.
type Detection struct {
	Kind  Kind   `json:"kind"`
	Label string `json:"label"`
	PID   int    `json:"pid"`
	// Transcripts reports whether transcripts can be read for this agent.
	Transcripts bool `json:"transcripts"`
}

// PSCommand is the portable process listing used for detection. The trailing
// "=" suppresses headers on both Linux procps and BSD/macOS ps.
const PSCommand = "ps -eo pid=,ppid=,args="

// ParsePS parses the output of PSCommand. Malformed lines are skipped.
func ParsePS(output string) []Process {
	var procs []Process
	scanner := bufio.NewScanner(strings.NewReader(output))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 {
			continue
		}
		pid, err1 := strconv.Atoi(fields[0])
		ppid, err2 := strconv.Atoi(fields[1])
		if err1 != nil || err2 != nil {
			continue
		}
		procs = append(procs, Process{PID: pid, PPID: ppid, Args: strings.Join(fields[2:], " ")})
	}
	return procs
}

// FindAgent checks rootPID itself and then walks its descendants
// breadth-first, returning the first process that is a known agent. The root
// is included because a pane can run the agent directly (tmux started with
// the agent as its command, or a shell that exec'd it); breadth-first means
// the agent's main process wins over helpers it spawned itself.
func FindAgent(procs []Process, rootPID int) (Detection, bool) {
	if rootPID <= 0 {
		return Detection{}, false
	}
	children := make(map[int][]Process, len(procs))
	for _, p := range procs {
		children[p.PPID] = append(children[p.PPID], p)
		if p.PID == rootPID {
			if d, ok := MatchArgs(p.Args); ok {
				return Detection{Kind: d.Kind, Label: d.Label, PID: p.PID, Transcripts: d.Transcripts}, true
			}
		}
	}
	queue := []int{rootPID}
	seen := map[int]bool{rootPID: true}
	for len(queue) > 0 {
		pid := queue[0]
		queue = queue[1:]
		for _, child := range children[pid] {
			if seen[child.PID] {
				continue
			}
			seen[child.PID] = true
			if d, ok := MatchArgs(child.Args); ok {
				return Detection{Kind: d.Kind, Label: d.Label, PID: child.PID, Transcripts: d.Transcripts}, true
			}
			queue = append(queue, child.PID)
		}
	}
	return Detection{}, false
}

// MatchArgs checks a command line for a known agent binary. Only the first few
// tokens are inspected: the executable itself and, for interpreter-launched
// agents ("node .../bin/claude"), the script path.
func MatchArgs(args string) (Descriptor, bool) {
	tokens := strings.Fields(args)
	limit := 3
	if len(tokens) < limit {
		limit = len(tokens)
	}
	for _, tok := range tokens[:limit] {
		if strings.HasPrefix(tok, "-") {
			continue
		}
		base := path.Base(strings.ReplaceAll(tok, "\\", "/"))
		if d, ok := descriptorForBinary(base); ok {
			return d, true
		}
	}
	return Descriptor{}, false
}
