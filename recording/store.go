package recording

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	aggPinnedVersion = "v1.9.0"
	aggMaxBytes      = 32 * 1024 * 1024
	aggRenderTimeout = 2 * time.Minute
)

type aggAsset struct {
	name   string
	sha256 string
	size   int64
}

func pinnedAggAsset() (aggAsset, error) {
	switch runtime.GOOS {
	case "windows":
		if runtime.GOARCH == "amd64" {
			return aggAsset{
				name:   "agg-x86_64-pc-windows-msvc.exe",
				sha256: "810baf5506e74ca65d8ed85be3db58791086c8b7b0a17c9018d7fede473f0055",
				size:   14344704,
			}, nil
		}
	case "darwin":
		if runtime.GOARCH == "arm64" {
			return aggAsset{
				name:   "agg-aarch64-apple-darwin",
				sha256: "742b2b6230529b72f310acb835e9479496000f2eabc97b0993cabe1d7fe70171",
				size:   13754592,
			}, nil
		}
		if runtime.GOARCH == "amd64" {
			return aggAsset{
				name:   "agg-x86_64-apple-darwin",
				sha256: "1462150b611d231d2950d10a676303eaeb1019ff330735882aaae09b52e2e1c1",
				size:   15075896,
			}, nil
		}
	case "linux":
		if runtime.GOARCH == "arm64" {
			return aggAsset{
				name:   "agg-aarch64-unknown-linux-gnu",
				sha256: "2b4be407b97e00e1c313a41d154ced8fa3d02c560c8f47a0db4950a2576444c9",
				size:   13797992,
			}, nil
		}
		if runtime.GOARCH == "amd64" {
			return aggAsset{
				name:   "agg-x86_64-unknown-linux-gnu",
				sha256: "f111e315cd71056b116302342553dd765b7297579ed511f111d0cedb442aeda6",
				size:   15904064,
			}, nil
		}
	}
	return aggAsset{}, fmt.Errorf("recording: unsupported platform: %s/%s", runtime.GOOS, runtime.GOARCH)
}

func pinnedAggURL(asset string) string {
	return "https://github.com/asciinema/agg/releases/download/" + aggPinnedVersion + "/" + asset
}

// RecordingInfo holds metadata about a single recording.
type RecordingInfo struct {
	ID        string       `json:"id"`
	Title     string       `json:"title"`
	Timestamp int64        `json:"timestamp"`
	Duration  float64      `json:"duration"`
	Width     int          `json:"width"`
	Height    int          `json:"height"`
	Size      int64        `json:"size"`
	Meta      *SessionMeta `json:"meta,omitempty"`
}

// CutRegion defines a time range to be removed during trimmed export.
type CutRegion struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

// Store manages recordings on disk.
type Store struct {
	mu sync.Mutex
}

// NewStore creates a new recording store, ensuring the directory exists.
func NewStore() (*Store, error) {
	if _, err := recordingsDir(); err != nil {
		return nil, err
	}
	return &Store{}, nil
}

// List returns all recordings sorted by timestamp descending.
func (s *Store) List() ([]RecordingInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir, err := recordingsDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("recording: read dir: %w", err)
	}

	var list []RecordingInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".cast") {
			continue
		}

		id := strings.TrimSuffix(e.Name(), ".cast")
		info, err := parseRecordingInfo(filepath.Join(dir, e.Name()), id)
		if err != nil {
			continue
		}
		list = append(list, info)
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].Timestamp > list[j].Timestamp
	})

	return list, nil
}

// Get returns the full .cast file content for playback.
func (s *Store) Get(id string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	path, err := s.resolvePath(id)
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("recording: read file: %w", err)
	}
	return string(data), nil
}

// Delete removes a recording file.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path, err := s.resolvePath(id)
	if err != nil {
		return err
	}
	return os.Remove(path)
}

// ExportScrubbed returns the recording content with sensitive data removed.
func (s *Store) ExportScrubbed(id string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	path, err := s.resolvePath(id)
	if err != nil {
		return "", err
	}

	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("recording: open file: %w", err)
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)
	first := true
	for scanner.Scan() {
		line := scanner.Text()
		if first {
			// Header line — pass through (no terminal data)
			lines = append(lines, line)
			first = false
			continue
		}

		// Frame: [elapsed, type, data]
		var frame []json.RawMessage
		if err := json.Unmarshal([]byte(line), &frame); err != nil || len(frame) < 3 {
			lines = append(lines, line)
			continue
		}

		var eventType string
		if err := json.Unmarshal(frame[1], &eventType); err != nil {
			lines = append(lines, line)
			continue
		}

		if eventType == "o" || eventType == "i" {
			var data string
			if err := json.Unmarshal(frame[2], &data); err == nil {
				scrubbed := ScrubOutput(data)
				scrubbedJSON, _ := json.Marshal(scrubbed)
				frame[2] = scrubbedJSON
				newLine, _ := json.Marshal(frame)
				lines = append(lines, string(newLine))
				continue
			}
		}

		lines = append(lines, line)
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("recording: scan file: %w", err)
	}

	return strings.Join(lines, "\n") + "\n", nil
}

// ExportTrimmed returns the recording content with cut regions removed and timestamps adjusted.
// If scrub is true, sensitive data is also removed from the output.
func (s *Store) ExportTrimmed(id string, cuts []CutRegion, scrub bool) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	path, err := s.resolvePath(id)
	if err != nil {
		return "", err
	}

	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("recording: open file: %w", err)
	}
	defer f.Close()

	// Sort cuts by start time
	sortedCuts := make([]CutRegion, len(cuts))
	copy(sortedCuts, cuts)
	sort.Slice(sortedCuts, func(i, j int) bool {
		return sortedCuts[i].Start < sortedCuts[j].Start
	})

	var lines []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)
	first := true
	for scanner.Scan() {
		line := scanner.Text()
		if first {
			lines = append(lines, line)
			first = false
			continue
		}

		var frame []json.RawMessage
		if err := json.Unmarshal([]byte(line), &frame); err != nil || len(frame) < 3 {
			lines = append(lines, line)
			continue
		}

		var elapsed float64
		if err := json.Unmarshal(frame[0], &elapsed); err != nil {
			lines = append(lines, line)
			continue
		}

		// Check if this frame falls inside any cut region
		inCut := false
		for _, c := range sortedCuts {
			if elapsed >= c.Start && elapsed < c.End {
				inCut = true
				break
			}
		}
		if inCut {
			continue
		}

		// Calculate cumulative cut duration before this frame's timestamp
		var cutOffset float64
		for _, c := range sortedCuts {
			if c.End <= elapsed {
				cutOffset += c.End - c.Start
			} else if c.Start < elapsed {
				cutOffset += elapsed - c.Start
			}
		}

		// Adjust timestamp
		adjusted := elapsed - cutOffset
		adjustedJSON, _ := json.Marshal(adjusted)
		frame[0] = adjustedJSON

		// Optionally scrub output
		if scrub {
			var eventType string
			if err := json.Unmarshal(frame[1], &eventType); err == nil {
				if eventType == "o" || eventType == "i" {
					var data string
					if err := json.Unmarshal(frame[2], &data); err == nil {
						scrubbed := ScrubOutput(data)
						scrubbedJSON, _ := json.Marshal(scrubbed)
						frame[2] = scrubbedJSON
					}
				}
			}
		}

		newLine, _ := json.Marshal(frame)
		lines = append(lines, string(newLine))
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("recording: scan file: %w", err)
	}

	return strings.Join(lines, "\n") + "\n", nil
}

// ExportTrimmedGIF generates a GIF from a trimmed recording using `agg`.
func (s *Store) ExportTrimmedGIF(id string, cuts []CutRegion) (string, error) {
	tmpGif, err := os.CreateTemp("", "mimir-trimmed-*.gif")
	if err != nil {
		return "", fmt.Errorf("recording: create temp gif: %w", err)
	}
	gifPath := tmpGif.Name()
	if err := tmpGif.Close(); err != nil {
		os.Remove(gifPath)
		return "", fmt.Errorf("recording: close temp gif: %w", err)
	}
	if err := s.ExportTrimmedGIFTo(id, cuts, gifPath); err != nil {
		os.Remove(gifPath)
		return "", err
	}
	return gifPath, nil
}

// ExportTrimmedGIFTo generates a trimmed GIF at the selected output path.
func (s *Store) ExportTrimmedGIFTo(id string, cuts []CutRegion, gifPath string) error {
	trimmed, err := s.ExportTrimmed(id, cuts, false)
	if err != nil {
		return err
	}

	bin := aggPath()
	if bin == "" {
		return fmt.Errorf("recording: agg is not installed")
	}
	if strings.TrimSpace(gifPath) == "" {
		return fmt.Errorf("recording: gif output path is empty")
	}

	tmpFile, err := os.CreateTemp("", "mimir-trimmed-*.cast")
	if err != nil {
		return fmt.Errorf("recording: create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	if _, err := tmpFile.WriteString(trimmed); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("recording: write temp file: %w", err)
	}
	tmpFile.Close()

	if err := runAgg(bin, tmpPath, gifPath); err != nil {
		os.Remove(tmpPath)
		os.Remove(gifPath)
		return err
	}
	os.Remove(tmpPath)
	return nil
}

// ExportGIF generates a GIF from the recording using `agg` and returns the GIF path.
func (s *Store) ExportGIF(id string) (string, error) {
	tmpGif, err := os.CreateTemp("", "mimir-recording-*.gif")
	if err != nil {
		return "", fmt.Errorf("recording: create temp gif: %w", err)
	}
	gifPath := tmpGif.Name()
	if err := tmpGif.Close(); err != nil {
		os.Remove(gifPath)
		return "", fmt.Errorf("recording: close temp gif: %w", err)
	}
	if err := s.ExportGIFTo(id, gifPath); err != nil {
		os.Remove(gifPath)
		return "", err
	}
	return gifPath, nil
}

// ExportGIFTo generates a GIF from the recording at the selected output path.
func (s *Store) ExportGIFTo(id string, gifPath string) error {
	bin := aggPath()
	if bin == "" {
		return fmt.Errorf("recording: agg is not installed")
	}
	if strings.TrimSpace(gifPath) == "" {
		return fmt.Errorf("recording: gif output path is empty")
	}

	s.mu.Lock()
	path, err := s.resolvePath(id)
	s.mu.Unlock()
	if err != nil {
		return err
	}

	if err := runAgg(bin, path, gifPath); err != nil {
		os.Remove(gifPath)
		return err
	}
	return nil
}

func runAgg(bin, castPath, gifPath string) error {
	ctx, cancel := context.WithTimeout(context.Background(), aggRenderTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, castPath, gifPath)
	hideConsoleWindow(cmd)
	if output, err := cmd.CombinedOutput(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("recording: agg timed out after %s", aggRenderTimeout)
		}
		return fmt.Errorf("recording: agg failed: %s: %w", string(output), err)
	}
	return nil
}

// aggBinName returns the platform-specific agg binary name.
func aggBinName() string {
	if runtime.GOOS == "windows" {
		return "agg.exe"
	}
	return "agg"
}

// mimirBinDir returns the private mimir binary directory, creating it if needed.
func mimirBinDir() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("recording: config dir: %w", err)
	}
	dir := filepath.Join(configDir, "mimir", "bin")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("recording: create bin dir: %w", err)
	}
	return dir, nil
}

// aggPath returns the managed agg binary path. It intentionally does not fall
// back to PATH: recording conversion executes the returned binary directly.
func aggPath() string {
	if dir, err := mimirBinDir(); err == nil {
		local := filepath.Join(dir, aggBinName())
		if _, err := os.Stat(local); err == nil {
			return local
		}
	}
	return ""
}

// IsAggInstalled checks if the agg tool is available (local or PATH).
func (s *Store) IsAggInstalled() bool {
	return aggPath() != ""
}

// AggDownloadInfo contains details about the agg download for user confirmation.
type AggDownloadInfo struct {
	URL         string `json:"url"`
	Destination string `json:"destination"`
	Platform    string `json:"platform"`
	Version     string `json:"version"`
	SHA256      string `json:"sha256"`
	Size        int64  `json:"size"`
}

// GetAggDownloadInfo returns download details without actually downloading.
func (s *Store) GetAggDownloadInfo() (AggDownloadInfo, error) {
	asset, err := pinnedAggAsset()
	if err != nil {
		return AggDownloadInfo{}, err
	}

	dir, err := mimirBinDir()
	if err != nil {
		return AggDownloadInfo{}, err
	}

	return AggDownloadInfo{
		URL:         pinnedAggURL(asset.name),
		Destination: filepath.Join(dir, aggBinName()),
		Platform:    runtime.GOOS + "/" + runtime.GOARCH,
		Version:     aggPinnedVersion,
		SHA256:      asset.sha256,
		Size:        asset.size,
	}, nil
}

// DownloadAgg downloads the agg binary for the current platform to ~/.config/mimir/bin/.
func (s *Store) DownloadAgg() error {
	asset, err := pinnedAggAsset()
	if err != nil {
		return err
	}

	dir, err := mimirBinDir()
	if err != nil {
		return err
	}

	client := http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(pinnedAggURL(asset.name))
	if err != nil {
		return fmt.Errorf("recording: download agg: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("recording: download agg: HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > aggMaxBytes {
		return fmt.Errorf("recording: agg download too large: %d bytes", resp.ContentLength)
	}

	destPath := filepath.Join(dir, aggBinName())
	out, err := os.CreateTemp(dir, ".agg-*.tmp")
	if err != nil {
		return fmt.Errorf("recording: create temp file: %w", err)
	}
	tmpPath := out.Name()
	if err := out.Chmod(0600); err != nil {
		out.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("recording: chmod temp agg: %w", err)
	}

	hasher := sha256.New()
	limited := &io.LimitedReader{R: resp.Body, N: aggMaxBytes + 1}
	written, err := io.Copy(io.MultiWriter(out, hasher), limited)
	if err != nil {
		out.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("recording: write agg binary: %w", err)
	}
	if written > aggMaxBytes || limited.N == 0 {
		out.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("recording: agg download exceeds %d bytes", aggMaxBytes)
	}
	if asset.size > 0 && written != asset.size {
		out.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("recording: agg size mismatch: got %d bytes, expected %d", written, asset.size)
	}
	actualSHA := hex.EncodeToString(hasher.Sum(nil))
	if actualSHA != asset.sha256 {
		out.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("recording: agg checksum mismatch: got %s, expected %s", actualSHA, asset.sha256)
	}

	if err := out.Sync(); err != nil {
		out.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("recording: sync agg binary: %w", err)
	}
	if err := out.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("recording: close agg binary: %w", err)
	}

	if runtime.GOOS != "windows" {
		if err := os.Chmod(tmpPath, 0755); err != nil {
			os.Remove(tmpPath)
			return fmt.Errorf("recording: chmod agg: %w", err)
		}
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("recording: install agg: %w", err)
	}

	return nil
}

func (s *Store) resolvePath(id string) (string, error) {
	dir, err := recordingsDir()
	if err != nil {
		return "", err
	}

	// Sanitize ID to prevent path traversal
	base := filepath.Base(id)
	if base != id || strings.Contains(id, "..") {
		return "", fmt.Errorf("recording: invalid id")
	}

	path := filepath.Join(dir, id+".cast")
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("recording: not found: %s", id)
	}
	return path, nil
}

func parseRecordingInfo(path, id string) (RecordingInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return RecordingInfo{}, err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return RecordingInfo{}, err
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	// Read header (first line)
	if !scanner.Scan() {
		return RecordingInfo{}, fmt.Errorf("recording: empty file")
	}

	var header AsciicastHeader
	if err := json.Unmarshal(scanner.Bytes(), &header); err != nil {
		return RecordingInfo{}, err
	}

	// Scan to last frame to determine duration
	var lastElapsed float64
	for scanner.Scan() {
		var frame []json.RawMessage
		if err := json.Unmarshal(scanner.Bytes(), &frame); err != nil || len(frame) < 1 {
			continue
		}
		var elapsed float64
		if err := json.Unmarshal(frame[0], &elapsed); err == nil {
			lastElapsed = elapsed
		}
	}

	return RecordingInfo{
		ID:        id,
		Title:     header.Title,
		Timestamp: header.Timestamp,
		Duration:  lastElapsed,
		Width:     header.Width,
		Height:    header.Height,
		Size:      stat.Size(),
		Meta:      header.MimirMeta,
	}, nil
}
