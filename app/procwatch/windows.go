//go:build windows

package procwatch

import (
	"encoding/binary"
	"hash/fnv"
	"net"
	"strconv"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The Windows sampler uses the native APIs only, no PowerShell per tick:
// Toolhelp32 for the process list (pid, parent, image name), GetProcessTimes
// for the creation time (the pid-reuse key), QueryFullProcessImageName for
// the full path, and iphlpapi's GetExtendedTcpTable for TCP connections with
// their owning pid. Command-line arguments are not read (that would need
// NtQueryInformationProcess), so attribution by command line never happens
// here: confidence is strong or weak, never medium. UDP is not sampled.

type windowsSampler struct {
	iphlpapi            *windows.LazyDLL
	getExtendedTcpTable *windows.LazyProc
}

// New returns the native sampler.
func New() (Sampler, error) {
	dll := windows.NewLazySystemDLL("iphlpapi.dll")
	return &windowsSampler{iphlpapi: dll, getExtendedTcpTable: dll.NewProc("GetExtendedTcpTable")}, nil
}

const (
	processQueryLimitedInformation = 0x1000
	tcpTableOwnerPidAll            = 5
	afInet                         = 2
	afInet6                        = 23
)

type mibTcpRowOwnerPid struct {
	State      uint32
	LocalAddr  uint32
	LocalPort  uint32
	RemoteAddr uint32
	RemotePort uint32
	OwningPid  uint32
}

type mibTcp6RowOwnerPid struct {
	LocalAddr     [16]byte
	LocalScopeID  uint32
	LocalPort     uint32
	RemoteAddr    [16]byte
	RemoteScopeID uint32
	RemotePort    uint32
	State         uint32
	OwningPid     uint32
}

var mibTcpStates = map[uint32]string{
	1: "CLOSED", 2: "LISTEN", 3: "SYN_SENT", 4: "SYN_RECV", 5: "ESTABLISHED", 6: "FIN_WAIT1",
	7: "FIN_WAIT2", 8: "CLOSE_WAIT", 9: "CLOSING", 10: "LAST_ACK", 11: "TIME_WAIT", 12: "DELETE_TCB",
}

// Snapshot lists the tree below root and the TCP connections its
// processes own.
func (s *windowsSampler) Snapshot(root int, opts Options) (Snapshot, error) {
	maxProcs := opts.MaxProcs
	if maxProcs <= 0 {
		maxProcs = defaultMaxProcsWindows
	}
	snap := Snapshot{At: time.Now(), Procs: map[Key]Proc{}, Conns: map[uint64]Conn{}}
	if root <= 0 {
		return snap, nil
	}
	all, err := listProcesses()
	if err != nil {
		return snap, err
	}
	children := map[int][]procEntry{}
	for _, p := range all {
		children[p.ppid] = append(children[p.ppid], p)
	}
	byPid := map[int]procEntry{}
	for _, p := range all {
		byPid[p.pid] = p
	}
	queue := []int{root}
	seen := map[int]bool{root: true}
	var tree []procEntry
	if r, ok := byPid[root]; ok {
		tree = append(tree, r)
	}
	for len(queue) > 0 {
		pid := queue[0]
		queue = queue[1:]
		for _, c := range children[pid] {
			if seen[c.pid] {
				continue
			}
			seen[c.pid] = true
			if len(tree) >= maxProcs {
				snap.Truncated = true
				break
			}
			tree = append(tree, c)
			queue = append(queue, c.pid)
		}
	}
	inTree := map[int]bool{}
	for _, p := range tree {
		created, startedAt, image := processDetails(p.pid)
		proc := Proc{PID: p.pid, PPID: p.ppid, Start: created, Cmdline: image, StartedAt: startedAt}
		if proc.Cmdline == "" {
			proc.Cmdline = p.exe
		}
		snap.Procs[Key{PID: p.pid, Start: created}] = proc
		inTree[p.pid] = true
	}
	for _, c := range s.tcpConnections() {
		if !inTree[c.PID] {
			continue
		}
		snap.Conns[c.Inode] = c
	}
	return snap, nil
}

const defaultMaxProcsWindows = 300

type procEntry struct {
	pid, ppid int
	exe       string
}

func listProcesses() ([]procEntry, error) {
	h, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(h)
	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	if err := windows.Process32First(h, &entry); err != nil {
		return nil, err
	}
	var out []procEntry
	for {
		out = append(out, procEntry{pid: int(entry.ProcessID), ppid: int(entry.ParentProcessID), exe: windows.UTF16ToString(entry.ExeFile[:])})
		if err := windows.Process32Next(h, &entry); err != nil {
			break
		}
	}
	return out, nil
}

// processDetails returns the creation time (FILETIME ticks, the pid-reuse
// key), the same as wall-clock time, and the full image path; zero / ""
// when the process cannot be opened.
func processDetails(pid int) (uint64, time.Time, string) {
	h, err := windows.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		return 0, time.Time{}, ""
	}
	defer windows.CloseHandle(h)
	var creation, exit, kernel, user windows.Filetime
	created := uint64(0)
	var startedAt time.Time
	if err := windows.GetProcessTimes(h, &creation, &exit, &kernel, &user); err == nil {
		created = uint64(creation.HighDateTime)<<32 | uint64(creation.LowDateTime)
		startedAt = time.Unix(0, creation.Nanoseconds())
	}
	buf := make([]uint16, syscall.MAX_LONG_PATH)
	size := uint32(len(buf))
	image := ""
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &size); err == nil {
		image = windows.UTF16ToString(buf[:size])
	}
	return created, startedAt, image
}

// tcpConnections reads the IPv4 and IPv6 TCP tables with owning pids.
func (s *windowsSampler) tcpConnections() []Conn {
	var out []Conn
	for _, af := range []uint32{afInet, afInet6} {
		buf := s.tcpTable(af)
		if len(buf) < 4 {
			continue
		}
		n := binary.LittleEndian.Uint32(buf[:4])
		rows := buf[4:]
		switch af {
		case afInet:
			size := int(unsafe.Sizeof(mibTcpRowOwnerPid{}))
			for i := 0; i < int(n) && (i+1)*size <= len(rows); i++ {
				r := (*mibTcpRowOwnerPid)(unsafe.Pointer(&rows[i*size]))
				c := Conn{Proto: "tcp", PID: int(r.OwningPid), State: mibTcpStates[r.State],
					Local:  v4Addr(r.LocalAddr, r.LocalPort),
					Remote: v4Addr(r.RemoteAddr, r.RemotePort)}
				c.Inode = connKey(c)
				out = append(out, c)
			}
		case afInet6:
			size := int(unsafe.Sizeof(mibTcp6RowOwnerPid{}))
			for i := 0; i < int(n) && (i+1)*size <= len(rows); i++ {
				r := (*mibTcp6RowOwnerPid)(unsafe.Pointer(&rows[i*size]))
				c := Conn{Proto: "tcp6", PID: int(r.OwningPid), State: mibTcpStates[r.State],
					Local:  v6Addr(r.LocalAddr, r.LocalPort),
					Remote: v6Addr(r.RemoteAddr, r.RemotePort)}
				c.Inode = connKey(c)
				out = append(out, c)
			}
		}
	}
	return out
}

func (s *windowsSampler) tcpTable(af uint32) []byte {
	if s.getExtendedTcpTable.Find() != nil {
		return nil
	}
	var size uint32
	// First call sizes the buffer; the second fills it. Retry once if the
	// table grew in between.
	for attempt := 0; attempt < 3; attempt++ {
		buf := make([]byte, size+1)
		ret, _, _ := s.getExtendedTcpTable.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)), 0, uintptr(af), tcpTableOwnerPidAll, 0)
		switch ret {
		case 0:
			return buf
		case 122: // ERROR_INSUFFICIENT_BUFFER
			continue
		default:
			return nil
		}
	}
	return nil
}

func v4Addr(addr, port uint32) string {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], addr)
	return net.JoinHostPort(net.IP(b[:]).String(), strconv.Itoa(ntohs(port)))
}

func v6Addr(addr [16]byte, port uint32) string {
	return net.JoinHostPort(net.IP(addr[:]).String(), strconv.Itoa(ntohs(port)))
}

// ntohs: the port sits in the low 16 bits in network byte order.
func ntohs(v uint32) int {
	return int(uint16(v>>8&0xff) | uint16(v&0xff)<<8)
}

// connKey stands in for the inode Linux has: a hash of pid and endpoints.
func connKey(c Conn) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(strconv.Itoa(c.PID) + "|" + c.Proto + "|" + c.Local + "|" + c.Remote))
	return h.Sum64()
}
