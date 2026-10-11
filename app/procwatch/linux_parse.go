package procwatch

import (
	"encoding/binary"
	"net"
	"strconv"
	"strings"
)

// parseStat extracts ppid and starttime (field 22) from /proc/<pid>/stat.
// Fields after the parenthesised command name are counted from the state
// field, so starttime is rest[19].
func parseStat(line string) (ppid int, start uint64, ok bool) {
	close := strings.LastIndexByte(line, ')')
	if close < 0 {
		return 0, 0, false
	}
	rest := strings.Fields(line[close+1:])
	if len(rest) < 20 {
		return 0, 0, false
	}
	ppid, err := strconv.Atoi(rest[1])
	if err != nil {
		return 0, 0, false
	}
	start, err = strconv.ParseUint(rest[19], 10, 64)
	if err != nil {
		return 0, 0, false
	}
	return ppid, start, true
}

// parseCmdline turns the NUL-separated /proc/<pid>/cmdline into one line,
// capped at max bytes (0 = no cap).
func parseCmdline(data []byte, max int) string {
	s := strings.TrimRight(string(data), "\x00")
	s = strings.ReplaceAll(s, "\x00", " ")
	if max > 0 && len(s) > max {
		s = s[:max] + "...[truncated]"
	}
	return s
}

// parseSocketInode reads the inode of a "socket:[12345]" fd link.
func parseSocketInode(link string) (uint64, bool) {
	if !strings.HasPrefix(link, "socket:[") || !strings.HasSuffix(link, "]") {
		return 0, false
	}
	n, err := strconv.ParseUint(link[len("socket:["):len(link)-1], 10, 64)
	return n, err == nil
}

// tcpStates maps the hex state of /proc/net/tcp to its name.
var tcpStates = map[string]string{
	"01": "ESTABLISHED", "02": "SYN_SENT", "03": "SYN_RECV", "04": "FIN_WAIT1", "05": "FIN_WAIT2",
	"06": "TIME_WAIT", "07": "CLOSE", "08": "CLOSE_WAIT", "09": "LAST_ACK", "0A": "LISTEN", "0B": "CLOSING",
}

// netEntry is one line of /proc/net/tcp{,6} or udp{,6}.
type netEntry struct {
	Inode  uint64
	Local  string
	Remote string
	State  string
}

// parseNetTable parses a /proc/net/{tcp,tcp6,udp,udp6} table. The header
// line is skipped. Addresses are kernel hex: little-endian 32-bit words,
// one for IPv4 and four for IPv6.
func parseNetTable(data string, proto string) []netEntry {
	var out []netEntry
	for i, line := range strings.Split(data, "\n") {
		if i == 0 {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 10 {
			continue
		}
		inode, err := strconv.ParseUint(f[9], 10, 64)
		if err != nil || inode == 0 {
			continue
		}
		e := netEntry{Inode: inode, Local: hexAddr(f[1]), Remote: hexAddr(f[2])}
		if strings.HasPrefix(proto, "tcp") {
			e.State = tcpStates[strings.ToUpper(f[3])]
		}
		out = append(out, e)
	}
	return out
}

// hexAddr decodes "0100007F:0BB8" to "127.0.0.1:3000" and the 32-char
// IPv6 form to "[::1]:3000".
func hexAddr(s string) string {
	host, port, ok := strings.Cut(s, ":")
	if !ok {
		return s
	}
	p, err := strconv.ParseUint(port, 16, 16)
	if err != nil {
		return s
	}
	switch len(host) {
	case 8:
		v, err := strconv.ParseUint(host, 16, 32)
		if err != nil {
			return s
		}
		var b [4]byte
		binary.LittleEndian.PutUint32(b[:], uint32(v))
		return net.JoinHostPort(net.IP(b[:]).String(), strconv.FormatUint(p, 10))
	case 32:
		var ip [16]byte
		for w := 0; w < 4; w++ {
			v, err := strconv.ParseUint(host[w*8:(w+1)*8], 16, 32)
			if err != nil {
				return s
			}
			binary.LittleEndian.PutUint32(ip[w*4:(w+1)*4], uint32(v))
		}
		addr := net.IP(ip[:])
		if v4 := addr.To4(); v4 != nil && strings.HasPrefix(addr.String(), "::ffff:") {
			return net.JoinHostPort(v4.String(), strconv.FormatUint(p, 10))
		}
		return net.JoinHostPort(addr.String(), strconv.FormatUint(p, 10))
	default:
		return s
	}
}

// parseChildren reads /proc/<pid>/task/<pid>/children ("12 34 56 ").
func parseChildren(data string) []int {
	var out []int
	for _, f := range strings.Fields(data) {
		if n, err := strconv.Atoi(f); err == nil && n > 0 {
			out = append(out, n)
		}
	}
	return out
}
