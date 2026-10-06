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
	// Elapsed and CPU are only filled by ParsePSDetailed.
	Elapsed string
	CPU     string
	Args    string
}

// Detection is the result of scanning a terminal's process tree.
type Detection struct {
	Kind  Kind   `json:"kind"`
	Label string `json:"label"`
	Short string `json:"short"`
	PID   int    `json:"pid"`
	// Transcripts reports whether transcripts can be read for this agent.
	Transcripts bool `json:"transcripts"`
}

// PSCommand is the portable process listing used for detection. The trailing
// "=" suppresses headers on both Linux procps and BSD/macOS ps.
const PSCommand = "ps -eo pid=,ppid=,args="

// PSDetailedCommand adds elapsed time and CPU share for the process view.
const PSDetailedCommand = "ps -eo pid=,ppid=,etime=,pcpu=,args="

// ParsePSDetailed parses PSDetailedCommand output (or the same five
// tab/space separated columns from another source).
func ParsePSDetailed(output string) []Process {
	var procs []Process
	scanner := bufio.NewScanner(strings.NewReader(output))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 5 {
			continue
		}
		pid, err1 := strconv.Atoi(fields[0])
		ppid, err2 := strconv.Atoi(fields[1])
		if err1 != nil || err2 != nil {
			continue
		}
		procs = append(procs, Process{PID: pid, PPID: ppid, Elapsed: fields[2], CPU: fields[3], Args: strings.Join(fields[4:], " ")})
	}
	return procs
}

// ProcessNode is one row of the agent's process tree.
type ProcessNode struct {
	PID     int    `json:"pid"`
	PPID    int    `json:"ppid"`
	Depth   int    `json:"depth"`
	Args    string `json:"args"`
	Elapsed string `json:"elapsed,omitempty"`
	CPU     string `json:"cpu,omitempty"`
}

// Subtree returns rootPID and its descendants in tree order (depth first,
// children in table order). Empty when rootPID is not in the table.
func Subtree(procs []Process, rootPID int) []ProcessNode {
	byPID := make(map[int]Process, len(procs))
	children := make(map[int][]int, len(procs))
	for _, p := range procs {
		byPID[p.PID] = p
		children[p.PPID] = append(children[p.PPID], p.PID)
	}
	root, ok := byPID[rootPID]
	if !ok {
		return nil
	}
	var out []ProcessNode
	seen := map[int]bool{}
	var walk func(p Process, depth int)
	walk = func(p Process, depth int) {
		if seen[p.PID] || len(out) > 500 {
			return
		}
		seen[p.PID] = true
		out = append(out, ProcessNode{PID: p.PID, PPID: p.PPID, Depth: depth, Args: p.Args, Elapsed: p.Elapsed, CPU: p.CPU})
		for _, c := range children[p.PID] {
			walk(byPID[c], depth+1)
		}
	}
	walk(root, 0)
	return out
}

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
				return Detection{Kind: d.Kind, Label: d.Label, Short: d.Short, PID: p.PID, Transcripts: d.Transcripts}, true
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
				return Detection{Kind: d.Kind, Label: d.Label, Short: d.Short, PID: child.PID, Transcripts: d.Transcripts}, true
			}
			queue = append(queue, child.PID)
		}
	}
	return Detection{}, false
}

// SplitCommandLine is splitCommandLine for callers outside the package.
func SplitCommandLine(args string) []string { return splitCommandLine(args) }

// splitCommandLine splits a command line into tokens, honouring double
// quotes the way Windows command lines use them (`"C:\\Program Files\\x"`),
// which strings.Fields would cut at the space.
func splitCommandLine(args string) []string {
	var out []string
	var cur strings.Builder
	inQuote, has := false, false
	for _, r := range args {
		switch {
		case r == '"':
			inQuote = !inQuote
			has = true
		case !inQuote && (r == ' ' || r == '\t' || r == '\n' || r == '\r'):
			if has {
				out = append(out, cur.String())
				cur.Reset()
				has = false
			}
		default:
			cur.WriteRune(r)
			has = true
		}
	}
	if has {
		out = append(out, cur.String())
	}
	return out
}

// interpreters are launchers whose first non-flag argument is the program
// that actually runs ("node .../bin/claude", "python3 -m aider").
var interpreters = map[string]bool{
	"node": true, "nodejs": true, "bun": true, "deno": true,
	"python": true, "python3": true, "py": true,
	"cmd": true, "sh": true, "bash": true, "zsh": true,
}

// MatchArgs checks a command line for a known agent binary: the executable
// itself, or the script after an interpreter. Later arguments are not
// looked at, so "ssh hermes" or "less codex" are not agents.
func MatchArgs(args string) (Descriptor, bool) {
	tokens := splitCommandLine(args)
	if len(tokens) == 0 {
		return Descriptor{}, false
	}
	base := func(tok string) string {
		return path.Base(strings.ReplaceAll(tok, "\\", "/"))
	}
	exe := base(tokens[0])
	if d, ok := descriptorForBinary(exe); ok {
		return d, true
	}
	if !interpreters[strings.ToLower(strings.TrimSuffix(exe, ".exe"))] {
		return Descriptor{}, false
	}
	// The first non-flag token after the interpreter ("-m aider" counts).
	for _, tok := range tokens[1:] {
		if strings.HasPrefix(tok, "-") {
			continue
		}
		if d, ok := descriptorForBinary(base(tok)); ok {
			return d, true
		}
		return descriptorForScriptPath(tok)
	}
	return Descriptor{}, false
}

// genericScripts are entry points whose name says nothing; the npm
// package directory above them does ("@anthropic-ai/claude-code/cli.js").
var genericScripts = map[string]bool{"cli.js": true, "cli.mjs": true, "index.js": true, "index.mjs": true, "main.js": true, "bin.js": true}

var packageDirs = map[string]Kind{
	"claude-code": KindClaude,
	"codex":       KindCodex,
	"gemini-cli":  KindGemini,
	"opencode":    KindOpenCode,
	"opencode-ai": KindOpenCode,
}

func descriptorForScriptPath(tok string) (Descriptor, bool) {
	norm := strings.ReplaceAll(tok, "\\", "/")
	if !genericScripts[strings.ToLower(path.Base(norm))] {
		return Descriptor{}, false
	}
	for _, seg := range strings.Split(strings.ToLower(norm), "/") {
		if kind, ok := packageDirs[seg]; ok {
			return Lookup(kind)
		}
	}
	return Descriptor{}, false
}
