package resources

import (
	"strconv"
	"strings"
)

// parseProcStat parses one /proc/<pid>/stat line. Fields after the
// parenthesised command name: state, ppid, ..., utime (14th overall),
// stime (15th), ..., rss in pages (24th).
func parseProcStat(line string, pageSize int64, clkTck float64) (Sample, bool) {
	close := strings.LastIndexByte(line, ')')
	if close < 0 {
		return Sample{}, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(line[:strings.IndexByte(line, '(')]))
	if err != nil {
		return Sample{}, false
	}
	rest := strings.Fields(line[close+1:])
	if len(rest) < 22 {
		return Sample{}, false
	}
	ppid, err := strconv.Atoi(rest[1])
	if err != nil {
		return Sample{}, false
	}
	utime, _ := strconv.ParseFloat(rest[11], 64)
	stime, _ := strconv.ParseFloat(rest[12], 64)
	rssPages, _ := strconv.ParseInt(rest[21], 10, 64)
	return Sample{PID: pid, PPID: ppid, RSS: rssPages * pageSize, CPUSeconds: (utime + stime) / clkTck}, true
}

// parsePS parses `ps -axo pid=,ppid=,rss=,time=` output (rss in KB, time
// as [[dd-]hh:]mm:ss[.xx]).
func parsePS(out string) []Sample {
	var samples []Sample
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 4 {
			continue
		}
		pid, err1 := strconv.Atoi(f[0])
		ppid, err2 := strconv.Atoi(f[1])
		rssKB, err3 := strconv.ParseInt(f[2], 10, 64)
		if err1 != nil || err2 != nil || err3 != nil {
			continue
		}
		samples = append(samples, Sample{PID: pid, PPID: ppid, RSS: rssKB * 1024, CPUSeconds: parsePSTime(f[3])})
	}
	return samples
}

// parsePSTime converts "[[dd-]hh:]mm:ss[.xx]" to seconds.
func parsePSTime(s string) float64 {
	days := 0.0
	if i := strings.IndexByte(s, '-'); i >= 0 {
		d, _ := strconv.ParseFloat(s[:i], 64)
		days = d
		s = s[i+1:]
	}
	parts := strings.Split(s, ":")
	total := 0.0
	for _, p := range parts {
		v, _ := strconv.ParseFloat(p, 64)
		total = total*60 + v
	}
	return days*86400 + total
}

// parseTSV parses "pid\tppid\trssBytes\tcpu100ns" lines (Windows CIM).
func parseTSV(out string) []Sample {
	var samples []Sample
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(strings.TrimRight(line, "\r"), "\t")
		if len(f) < 4 {
			continue
		}
		pid, err1 := strconv.Atoi(strings.TrimSpace(f[0]))
		ppid, err2 := strconv.Atoi(strings.TrimSpace(f[1]))
		rss, err3 := strconv.ParseInt(strings.TrimSpace(f[2]), 10, 64)
		cpu, err4 := strconv.ParseFloat(strings.TrimSpace(f[3]), 64)
		if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
			continue
		}
		samples = append(samples, Sample{PID: pid, PPID: ppid, RSS: rss, CPUSeconds: cpu / 1e7})
	}
	return samples
}
