package transcript

import (
	"encoding/json"
	"fmt"
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

// appendLocks holds one mutex per resumeID. POSIX guarantees O_APPEND writes
// are atomic up to PIPE_BUF (≈4 KB) — anything larger, or running on Windows
// where the guarantee doesn't apply at all, can interleave. Holding a mutex
// per resumeID for the duration of open/write/close removes the question
// entirely without serializing writes across different transcripts.
var appendLocks sync.Map

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

func ReadTail(resumeID string, maxBytes int) (string, error) {
	path, err := PathForResumeID(resumeID)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("failed to read transcript: %w", err)
	}
	if maxBytes > 0 && len(data) > maxBytes {
		data = data[len(data)-maxBytes:]
	}
	return string(data), nil
}

// ReadFull returns the entire transcript for the given resume ID. When
// maxBytes is positive and the file exceeds it, the head is truncated and the
// last maxBytes bytes are returned — callers that need the full file regardless
// of size should pass 0.
func ReadFull(resumeID string, maxBytes int) (string, error) {
	return ReadTail(resumeID, maxBytes)
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
