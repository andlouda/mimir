package transcript

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"mimir/safeio"
)

// DefaultMaxFileSize caps any single transcript file at 100 MiB. Long-running
// log-tail sessions can produce hundreds of MB of output per day; without a
// cap they fill the user config volume. The cap is enforced in the Append
// path, not retroactively — existing files larger than the cap are still
// readable, they just stop growing.
const DefaultMaxFileSize int64 = 100 * 1024 * 1024

var (
	maxFileSizeMu sync.RWMutex
	maxFileSize   = DefaultMaxFileSize
)

// SetMaxFileSize overrides the per-file cap. Pass DefaultMaxFileSize to
// restore the default. Passing 0 disables the cap (unbounded growth — only
// useful for tests).
func SetMaxFileSize(n int64) {
	maxFileSizeMu.Lock()
	maxFileSize = n
	maxFileSizeMu.Unlock()
}

func currentMaxFileSize() int64 {
	maxFileSizeMu.RLock()
	defer maxFileSizeMu.RUnlock()
	return maxFileSize
}

// appendLocks holds one mutex per resumeID. POSIX guarantees O_APPEND writes
// are atomic up to PIPE_BUF (≈4 KB) — anything larger, or running on Windows
// where the guarantee doesn't apply at all, can interleave. Holding a mutex
// per resumeID for the duration of open/write/close removes the question
// entirely without serializing writes across different transcripts.
var appendLocks sync.Map

// appendDropLog rate-limits the "file at cap, dropping writes" log message
// to one per resumeID per limitDropLogInterval, so a runaway producer doesn't
// flood the app log.
var (
	appendDropLog            sync.Map // resumeID -> *time.Time
	limitDropLogInterval     = 1 * time.Minute
)

func appendLockFor(resumeID string) *sync.Mutex {
	m, _ := appendLocks.LoadOrStore(resumeID, &sync.Mutex{})
	return m.(*sync.Mutex)
}

// Metadata is the side-car descriptor written next to a transcript so the
// terminal's label survives the in-memory session and the saved snapshot.
// Without this, every closed terminal becomes anonymous on next launch.
type Metadata struct {
	Name         string    `json:"name,omitempty"`
	Type         string    `json:"type,omitempty"`
	SSHProfileID string    `json:"sshProfileId,omitempty"`
	StartedAt    time.Time `json:"startedAt,omitempty"`
	UpdatedAt    time.Time `json:"updatedAt,omitempty"`
}

// Entry describes a stored transcript discoverable via List.
type Entry struct {
	ResumeID string    `json:"resumeId"`
	Size     int64     `json:"size"`
	ModTime  time.Time `json:"modTime"`
	Metadata Metadata  `json:"metadata"`
}

// Content is the authoritative result of a read. Frontends must NOT infer
// truncation from string-length math (UTF-8 byte-count and JS char-length
// differ) — they consult Truncated and ReadBytes here.
type Content struct {
	ResumeID  string `json:"resumeId"`
	Text      string `json:"text"`
	Size      int64  `json:"size"`      // total file size in bytes
	ReadBytes int64  `json:"readBytes"` // bytes actually included in Text
	Truncated bool   `json:"truncated"` // true iff ReadBytes < Size
}

var resumeIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

func transcriptsDir() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user config directory: %w", err)
	}
	dir := filepath.Join(configDir, "mimir", "transcripts")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("failed to create transcript directory: %w", err)
	}
	return dir, nil
}

func PathForResumeID(resumeID string) (string, error) {
	if resumeID == "" {
		return "", fmt.Errorf("resume id is required")
	}
	if !resumeIDPattern.MatchString(resumeID) {
		return "", fmt.Errorf("invalid resume id")
	}
	dir, err := transcriptsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, resumeID+".log"), nil
}

func metadataPath(resumeID string) (string, error) {
	if resumeID == "" {
		return "", fmt.Errorf("resume id is required")
	}
	if !resumeIDPattern.MatchString(resumeID) {
		return "", fmt.Errorf("invalid resume id")
	}
	dir, err := transcriptsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, resumeID+".json"), nil
}

// WriteMetadata persists the descriptor next to the transcript. If a previous
// descriptor exists the StartedAt is preserved and only UpdatedAt is bumped —
// callers don't need to track which is the first write themselves.
//
// When the existing descriptor already carries the exact same Name / Type /
// SSHProfileID, the disk write is skipped and the previous file is kept
// verbatim. This matters because the frontend fires a metadata write on every
// rename event, and most renames are no-ops in practice (re-saving the same
// name closes the inline-edit input).
func WriteMetadata(resumeID string, meta Metadata) error {
	path, err := metadataPath(resumeID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if existing, err := os.ReadFile(path); err == nil {
		var prev Metadata
		if err := json.Unmarshal(existing, &prev); err == nil {
			if !prev.StartedAt.IsZero() {
				meta.StartedAt = prev.StartedAt
			}
			if prev.Name == meta.Name &&
				prev.Type == meta.Type &&
				prev.SSHProfileID == meta.SSHProfileID {
				return nil
			}
		}
	}
	if meta.StartedAt.IsZero() {
		meta.StartedAt = now
	}
	meta.UpdatedAt = now

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal transcript metadata: %w", err)
	}
	// Atomic write via safeio: write to a sibling temp file, fsync, rename
	// into place. A crash mid-write leaves either the previous side-car
	// untouched or no file at all — never a half-written JSON that
	// ReadMetadata would mark as corrupt.
	if err := safeio.AtomicWriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("failed to write transcript metadata: %w", err)
	}
	return nil
}

// ReadMetadata loads the descriptor written by WriteMetadata. Missing files
// return a zero Metadata without error so callers can treat absence as the
// pre-existing default.
func ReadMetadata(resumeID string) (Metadata, error) {
	path, err := metadataPath(resumeID)
	if err != nil {
		return Metadata{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Metadata{}, nil
		}
		return Metadata{}, fmt.Errorf("failed to read transcript metadata: %w", err)
	}
	var meta Metadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return Metadata{}, fmt.Errorf("failed to parse transcript metadata: %w", err)
	}
	return meta, nil
}

func Append(resumeID string, data string) (string, error) {
	if data == "" {
		return PathForResumeID(resumeID)
	}
	path, err := PathForResumeID(resumeID)
	if err != nil {
		return "", err
	}

	// Per-resumeID lock: parallel appends to the same transcript serialize,
	// appends to *different* transcripts run in parallel as before. The
	// lock covers open+write+close so a torn write is impossible even when
	// the OS doesn't guarantee O_APPEND atomicity for the given size.
	lock := appendLockFor(resumeID)
	lock.Lock()
	defer lock.Unlock()

	// Enforce the per-file cap. We stat under the lock so a racing append
	// can't both pass the check. Over-cap writes are silently dropped
	// (fire-and-forget callers can't react anyway) with a rate-limited log.
	if cap := currentMaxFileSize(); cap > 0 {
		if info, err := os.Stat(path); err == nil {
			if info.Size() >= cap {
				logDroppedAppend(resumeID, info.Size(), cap)
				return path, nil
			}
		}
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return "", fmt.Errorf("failed to open transcript file: %w", err)
	}
	defer f.Close()
	if _, err := f.WriteString(data); err != nil {
		return "", fmt.Errorf("failed to append transcript: %w", err)
	}
	return path, nil
}

func logDroppedAppend(resumeID string, currentSize, cap int64) {
	now := time.Now()
	if last, ok := appendDropLog.Load(resumeID); ok {
		if t, ok := last.(time.Time); ok && now.Sub(t) < limitDropLogInterval {
			return
		}
	}
	appendDropLog.Store(resumeID, now)
	// Log resume ID (opaque UUID), not transcript content.
	log.Printf("transcript: %s reached size cap (%d/%d bytes), dropping new writes", resumeID, currentSize, cap)
}

func ReadTail(resumeID string, maxBytes int) (string, error) {
	path, err := PathForResumeID(resumeID)
	if err != nil {
		return "", err
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("failed to open transcript: %w", err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return "", fmt.Errorf("failed to stat transcript: %w", err)
	}
	size := info.Size()

	// maxBytes <= 0 means "no cap": read everything. Otherwise tail-seek so
	// we don't drag 50 MB through memory just to keep the last 16 KB. The
	// previous implementation read the whole file and then sliced — fine for
	// correctness, expensive on the hot restore path.
	start := int64(0)
	if maxBytes > 0 && size > int64(maxBytes) {
		start = size - int64(maxBytes)
	}
	if start > 0 {
		if _, err := f.Seek(start, io.SeekStart); err != nil {
			return "", fmt.Errorf("failed to seek transcript: %w", err)
		}
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return "", fmt.Errorf("failed to read transcript: %w", err)
	}

	// UTF-8 boundary: if we sliced the file inside a multi-byte rune, the
	// first 1-3 bytes are continuation bytes (0b10xxxxxx). Skipping them
	// keeps the returned string valid UTF-8 and only shaves at most 3 bytes
	// off the requested window.
	if start > 0 {
		data = trimLeadingUTF8Continuations(data)
	}
	return string(data), nil
}

// trimLeadingUTF8Continuations drops bytes at the front of b that are UTF-8
// continuation bytes (high bits 10). After at most 3 byte-drops the first
// remaining byte is either ASCII or the start of a valid multi-byte rune.
func trimLeadingUTF8Continuations(b []byte) []byte {
	for i := 0; i < len(b) && i < 3; i++ {
		if b[0]&0xC0 != 0x80 {
			break
		}
		b = b[1:]
	}
	return b
}

// ReadFull returns the entire transcript for the given resume ID. When
// maxBytes is positive and the file exceeds it, the head is truncated and the
// last maxBytes bytes are returned — callers that need the full file regardless
// of size should pass 0.
//
// Deprecated for the Wails layer: prefer ReadContent for clients that need
// to distinguish "fits in the cap" from "was truncated". ReadFull stays for
// backwards-compat with internal callers (tests, restore-overlay excerpt).
func ReadFull(resumeID string, maxBytes int) (string, error) {
	return ReadTail(resumeID, maxBytes)
}

// ReadContent is the authoritative read API. It returns the file content,
// the on-disk size, the number of bytes actually included, and a Truncated
// flag — frontends rely on Truncated rather than guessing from string length.
// maxBytes <= 0 means "no cap" (in practice limited by the caller's hard cap).
func ReadContent(resumeID string, maxBytes int) (Content, error) {
	path, err := PathForResumeID(resumeID)
	if err != nil {
		return Content{ResumeID: resumeID}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Content{ResumeID: resumeID}, nil
		}
		return Content{ResumeID: resumeID}, fmt.Errorf("failed to stat transcript: %w", err)
	}

	text, err := ReadTail(resumeID, maxBytes)
	if err != nil {
		return Content{ResumeID: resumeID, Size: info.Size()}, err
	}
	read := int64(len(text))
	return Content{
		ResumeID:  resumeID,
		Text:      text,
		Size:      info.Size(),
		ReadBytes: read,
		Truncated: read < info.Size(),
	}, nil
}

// DeleteResult reports per-resumeID outcome for batch deletions. Bulk callers
// receive one entry per requested ID so they can render mixed success / skip
// outcomes without losing track of which deletion failed and why.
type DeleteResult struct {
	ResumeID string `json:"resumeId"`
	OK       bool   `json:"ok"`
	// Reason is "active" (skipped because the terminal is still open),
	// "not_found" (no .log on disk), "invalid_id", "io_error" (underlying
	// filesystem error). Empty when OK==true.
	Reason string `json:"reason,omitempty"`
	Error  string `json:"error,omitempty"`
}

// DiskUsageInfo is the aggregate over all stored transcripts (log files only;
// side-car JSON is excluded so the number tracks user-visible content size).
type DiskUsageInfo struct {
	Count      int   `json:"count"`
	TotalBytes int64 `json:"totalBytes"`
}

// IsActiveFunc is the lookup the delete paths use to honor the
// "never delete a live terminal" invariant. Apps pass a callback closed over
// their active-state map. Pass nil to skip the check (only safe for tests).
type IsActiveFunc func(resumeID string) bool

// Delete removes the transcript log and its side-car. Missing files are not
// an error — Delete is idempotent. Path-safety is enforced via the same
// resumeID validation as Append/Read.
func Delete(resumeID string) error {
	logPath, err := PathForResumeID(resumeID)
	if err != nil {
		return err
	}
	metaPath, err := metadataPath(resumeID)
	if err != nil {
		return err
	}
	// Containment check: PathForResumeID already enforces this via the
	// regex, but verify the resolved path lives inside the transcripts dir.
	// Defense in depth against future path-resolution changes.
	if err := ensureInsideTranscriptsDir(logPath); err != nil {
		return err
	}

	// Hold the per-resumeID append lock so we don't race with a concurrent
	// Append.
	lock := appendLockFor(resumeID)
	lock.Lock()
	defer lock.Unlock()

	if err := os.Remove(logPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete transcript: %w", err)
	}
	if err := os.Remove(metaPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete transcript metadata: %w", err)
	}
	// Forget the lock entry so the map doesn't grow unbounded.
	appendLocks.Delete(resumeID)
	appendDropLog.Delete(resumeID)
	return nil
}

// DeleteMany applies Delete to each resumeID and returns a per-entry result.
// Active transcripts are skipped (per the isActive callback) with reason
// "active". Best-effort semantics — one failed delete doesn't abort the rest.
func DeleteMany(resumeIDs []string, isActive IsActiveFunc) []DeleteResult {
	out := make([]DeleteResult, 0, len(resumeIDs))
	for _, id := range resumeIDs {
		out = append(out, deleteOne(id, isActive))
	}
	return out
}

func deleteOne(resumeID string, isActive IsActiveFunc) DeleteResult {
	if resumeID == "" || !resumeIDPattern.MatchString(resumeID) {
		return DeleteResult{ResumeID: resumeID, Reason: "invalid_id"}
	}
	if isActive != nil && isActive(resumeID) {
		return DeleteResult{ResumeID: resumeID, Reason: "active"}
	}
	logPath, err := PathForResumeID(resumeID)
	if err != nil {
		return DeleteResult{ResumeID: resumeID, Reason: "invalid_id", Error: err.Error()}
	}
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		return DeleteResult{ResumeID: resumeID, Reason: "not_found"}
	}
	if err := Delete(resumeID); err != nil {
		return DeleteResult{ResumeID: resumeID, Reason: "io_error", Error: err.Error()}
	}
	return DeleteResult{ResumeID: resumeID, OK: true}
}

// DeleteOlderThan removes every transcript whose .log mtime is older than the
// cutoff. Active transcripts are always skipped. Returns the number of
// transcripts deleted.
func DeleteOlderThan(maxAge time.Duration, isActive IsActiveFunc) (int, error) {
	entries, err := List()
	if err != nil {
		return 0, err
	}
	cutoff := time.Now().Add(-maxAge)
	deleted := 0
	for _, entry := range entries {
		if entry.ModTime.After(cutoff) {
			continue
		}
		if isActive != nil && isActive(entry.ResumeID) {
			continue
		}
		if err := Delete(entry.ResumeID); err != nil {
			log.Printf("transcript: failed to delete %s during age-sweep: %v", entry.ResumeID, err)
			continue
		}
		deleted++
	}
	return deleted, nil
}

// DiskUsage returns the count and total byte size of every .log in the
// transcripts directory. Side-car JSON is not counted — the user is
// interested in the actual content footprint.
func DiskUsage() (DiskUsageInfo, error) {
	entries, err := List()
	if err != nil {
		return DiskUsageInfo{}, err
	}
	var total int64
	for _, e := range entries {
		total += e.Size
	}
	return DiskUsageInfo{Count: len(entries), TotalBytes: total}, nil
}

// ensureInsideTranscriptsDir verifies that the resolved target lives inside
// the transcripts directory. PathForResumeID already enforces this via the
// resumeID regex, but a defense-in-depth check at the delete boundary keeps
// us safe if validation ever loosens.
func ensureInsideTranscriptsDir(target string) error {
	dir, err := transcriptsDir()
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(dir, target)
	if err != nil {
		return fmt.Errorf("path outside transcripts dir: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path escapes transcripts dir: %s", target)
	}
	return nil
}

// List enumerates every stored transcript along with size and last-write time.
// Returned entries are sorted by ModTime descending so the freshest sessions
// appear first.
func List() ([]Entry, error) {
	dir, err := transcriptsDir()
	if err != nil {
		return nil, err
	}
	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read transcript directory: %w", err)
	}

	out := make([]Entry, 0, len(dirEntries))
	for _, de := range dirEntries {
		if de.IsDir() {
			continue
		}
		name := de.Name()
		if !strings.HasSuffix(name, ".log") {
			continue
		}
		resumeID := strings.TrimSuffix(name, ".log")
		if !resumeIDPattern.MatchString(resumeID) {
			continue
		}
		info, err := de.Info()
		if err != nil {
			continue
		}
		meta, metaErr := ReadMetadata(resumeID)
		if metaErr != nil {
			// Best-effort: list the transcript without metadata, but surface
			// the corruption so it doesn't stay hidden forever. The viewer
			// will fall back to "Closed terminal" labels for this entry.
			log.Printf("transcript: ignoring corrupt metadata for %s: %v", resumeID, metaErr)
		}
		out = append(out, Entry{
			ResumeID: resumeID,
			Size:     info.Size(),
			ModTime:  info.ModTime(),
			Metadata: meta,
		})
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].ModTime.After(out[j].ModTime)
	})
	return out, nil
}
