package timeline

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	// ringCap bounds the events kept in memory per session for the UI.
	ringCap = 5000
	// coldTailBytes is how much of a session file is read back when a
	// session is first touched after a restart.
	coldTailBytes = 1024 * 1024
	// maxNewUnboundPerHour caps session files created for events no pane
	// claimed (telemetry from Claude sessions outside Mimir).
	maxNewUnboundPerHour = 50
	fileSuffix           = ".jsonl"
)

// ErrTooManySessions is returned when the unbound-session cap is hit.
var ErrTooManySessions = errors.New("too many new sessions")

// Store keeps one JSONL file per session plus an in-memory ring of recent
// events, and tells subscribers about every appended event.
type Store struct {
	dir string

	mu        sync.Mutex
	seq       map[string]uint64
	rings     map[string][]Event
	terminals map[string]int // session -> terminal
	sessions  map[int]string // terminal -> session
	unbound   []time.Time
	subs      map[int]func(Event)
	nextSub   int
}

// NewStore creates a store below dir (created on first write, mode 0700).
func NewStore(dir string) *Store {
	return &Store{
		dir:       dir,
		seq:       map[string]uint64{},
		rings:     map[string][]Event{},
		terminals: map[string]int{},
		sessions:  map[int]string{},
		subs:      map[int]func(Event){},
	}
}

// Dir is the directory the session files live in.
func (s *Store) Dir() string { return s.dir }

// Path is the file of a session (not necessarily existing).
func (s *Store) Path(sessionID string) (string, error) {
	if !ValidSessionID(sessionID) {
		return "", ErrInvalidSession
	}
	return filepath.Join(s.dir, sessionID+fileSuffix), nil
}

// SetTerminal records which pane a session belongs to. The reverse index
// lets the UI ask for a pane's events; a pane that moves to another
// session drops the old link.
func (s *Store) SetTerminal(sessionID string, terminalID int) {
	if !ValidSessionID(sessionID) || terminalID <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if old, ok := s.sessions[terminalID]; ok && old != sessionID {
		delete(s.terminals, old)
	}
	s.terminals[sessionID] = terminalID
	s.sessions[terminalID] = sessionID
}

// TerminalFor is the pane bound to a session, 0 when none.
func (s *Store) TerminalFor(sessionID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.terminals[sessionID]
}

// SessionFor is the session bound to a pane, "" when none.
func (s *Store) SessionFor(terminalID int) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessions[terminalID]
}

// ForgetTerminal drops a pane's binding (pane closed).
func (s *Store) ForgetTerminal(terminalID int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.sessions[terminalID]; ok {
		delete(s.terminals, sess)
		delete(s.sessions, terminalID)
	}
}

// Subscribe registers a callback for every appended event (called outside
// the store's lock, on the appending goroutine). The result unsubscribes.
func (s *Store) Subscribe(fn func(Event)) func() {
	s.mu.Lock()
	id := s.nextSub
	s.nextSub++
	s.subs[id] = fn
	s.mu.Unlock()
	return func() {
		s.mu.Lock()
		delete(s.subs, id)
		s.mu.Unlock()
	}
}

// Append validates, numbers and persists an event, then notifies
// subscribers. An event without a terminal and for a session the store has
// never seen counts against the unbound cap. The sequence continues across
// restarts (read back from the file tail).
func (s *Store) Append(ev Event) (Event, error) {
	if ev.V == 0 {
		ev.V = SchemaVersion
	}
	if ev.TS == "" {
		ev.TS = FormatTS(time.Now())
	}
	if err := ev.Validate(); err != nil {
		return ev, err
	}
	file, _ := s.Path(ev.SessionID)

	s.mu.Lock()
	if ev.TerminalID == 0 {
		ev.TerminalID = s.terminals[ev.SessionID]
	}
	if _, known := s.seq[ev.SessionID]; !known {
		s.loadColdLocked(ev.SessionID, file)
		if _, known := s.seq[ev.SessionID]; !known {
			if ev.TerminalID == 0 && !s.allowUnboundLocked(time.Now()) {
				s.mu.Unlock()
				return ev, ErrTooManySessions
			}
			s.seq[ev.SessionID] = 0
		}
	}
	s.seq[ev.SessionID]++
	ev.Seq = s.seq[ev.SessionID]
	line, err := json.Marshal(ev)
	if err != nil {
		s.mu.Unlock()
		return ev, err
	}
	if err := appendLine(s.dir, file, line); err != nil {
		s.seq[ev.SessionID]--
		s.mu.Unlock()
		return ev, err
	}
	ring := append(s.rings[ev.SessionID], ev)
	if len(ring) > ringCap {
		ring = ring[len(ring)-ringCap:]
	}
	s.rings[ev.SessionID] = ring
	subs := make([]func(Event), 0, len(s.subs))
	for _, fn := range s.subs {
		subs = append(subs, fn)
	}
	s.mu.Unlock()

	for _, fn := range subs {
		fn(ev)
	}
	return ev, nil
}

// Recent returns up to limit events of a session with Seq > afterSeq, in
// order. afterSeq 0 and limit 0 mean "the newest ringCap events".
func (s *Store) Recent(sessionID string, afterSeq uint64, limit int) ([]Event, error) {
	file, err := s.Path(sessionID)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, known := s.seq[sessionID]; !known {
		s.loadColdLocked(sessionID, file)
	}
	ring := s.rings[sessionID]
	start := sort.Search(len(ring), func(i int) bool { return ring[i].Seq > afterSeq })
	out := ring[start:]
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return append([]Event(nil), out...), nil
}

// LastSeq is the newest sequence number of a session (0 = none).
func (s *Store) LastSeq(sessionID string) uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.seq[sessionID]
}

// Purge removes session files older than retention (by mtime; 0 keeps all)
// and rotates files above maxBytes to <file>.1 (0 = no rotation). Rings of
// rotated sessions are kept. Returns the number of files removed or
// rotated.
func (s *Store) Purge(retention time.Duration, maxBytes int64) int {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return 0
	}
	n := 0
	now := time.Now()
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, fileSuffix) && !strings.HasSuffix(name, fileSuffix+".1") {
			continue
		}
		p := filepath.Join(s.dir, name)
		info, err := e.Info()
		if err != nil {
			continue
		}
		if retention > 0 && now.Sub(info.ModTime()) > retention {
			if os.Remove(p) == nil {
				n++
				s.mu.Lock()
				sess := strings.TrimSuffix(strings.TrimSuffix(name, ".1"), fileSuffix)
				if strings.HasSuffix(name, fileSuffix) {
					delete(s.rings, sess)
					delete(s.seq, sess)
				}
				s.mu.Unlock()
			}
			continue
		}
		if maxBytes > 0 && strings.HasSuffix(name, fileSuffix) && info.Size() > maxBytes {
			s.mu.Lock()
			if os.Rename(p, p+".1") == nil {
				n++
			}
			s.mu.Unlock()
		}
	}
	return n
}

// allowUnboundLocked enforces the cap on sessions nobody claimed.
func (s *Store) allowUnboundLocked(now time.Time) bool {
	cutoff := now.Add(-time.Hour)
	kept := s.unbound[:0]
	for _, t := range s.unbound {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	s.unbound = kept
	if len(s.unbound) >= maxNewUnboundPerHour {
		return false
	}
	s.unbound = append(s.unbound, now)
	return true
}

// loadColdLocked reads the tail of an existing session file into the ring
// and continues its sequence. A missing file leaves the session unknown.
func (s *Store) loadColdLocked(sessionID, file string) {
	f, err := os.Open(file)
	if err != nil {
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return
	}
	offset := int64(0)
	if info.Size() > coldTailBytes {
		offset = info.Size() - coldTailBytes
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return
	}
	data, err := io.ReadAll(io.LimitReader(f, coldTailBytes))
	if err != nil {
		return
	}
	if offset > 0 {
		if i := bytes.IndexByte(data, '\n'); i >= 0 {
			data = data[i+1:]
		} else {
			data = nil
		}
	}
	var ring []Event
	var last uint64
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var ev Event
		if json.Unmarshal(sc.Bytes(), &ev) != nil || ev.SessionID != sessionID {
			continue
		}
		ring = append(ring, ev)
		if ev.Seq > last {
			last = ev.Seq
		}
	}
	if len(ring) > ringCap {
		ring = ring[len(ring)-ringCap:]
	}
	s.rings[sessionID] = ring
	s.seq[sessionID] = last
}

func appendLine(dir, file string, line []byte) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(file, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(line, '\n'))
	return err
}
