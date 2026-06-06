package transcript

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Entry describes a stored transcript discoverable via List.
type Entry struct {
	ResumeID string    `json:"resumeId"`
	Size     int64     `json:"size"`
	ModTime  time.Time `json:"modTime"`
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

func Append(resumeID string, data string) (string, error) {
	if data == "" {
		return PathForResumeID(resumeID)
	}
	path, err := PathForResumeID(resumeID)
	if err != nil {
		return "", err
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
		out = append(out, Entry{
			ResumeID: resumeID,
			Size:     info.Size(),
			ModTime:  info.ModTime(),
		})
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].ModTime.After(out[j].ModTime)
	})
	return out, nil
}
