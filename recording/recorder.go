package recording

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

// AsciicastHeader is the first line of an Asciicast v2 file.
type AsciicastHeader struct {
	Version   int               `json:"version"`
	Width     int               `json:"width"`
	Height    int               `json:"height"`
	Timestamp int64             `json:"timestamp"`
	Title     string            `json:"title,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	MimirMeta *SessionMeta      `json:"mimir,omitempty"`
}

// SessionMeta holds Mimir-specific metadata embedded in the recording header.
type SessionMeta struct {
	TerminalID  int    `json:"terminalId"`
	SessionType string `json:"sessionType,omitempty"`
	SSHHost     string `json:"sshHost,omitempty"`
	SSHProfile  string `json:"sshProfile,omitempty"`
}

// Recorder writes terminal output to an Asciicast v2 (.cast) file.
type Recorder struct {
	mu        sync.Mutex
	id        string
	file      *os.File
	startTime time.Time
	width     int
	height    int
	closed    bool
}

// NewRecorder creates a new recorder and writes the Asciicast v2 header.
func NewRecorder(width, height int, title string, meta *SessionMeta) (*Recorder, error) {
	dir, err := recordingsDir()
	if err != nil {
		return nil, err
	}

	id := uuid.New().String()
	path := filepath.Join(dir, id+".cast")

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return nil, fmt.Errorf("recording: create file: %w", err)
	}

	now := time.Now()
	header := AsciicastHeader{
		Version:   2,
		Width:     width,
		Height:    height,
		Timestamp: now.Unix(),
		Title:     title,
		Env:       map[string]string{"TERM": "xterm-256color"},
		MimirMeta: meta,
	}

	headerJSON, err := json.Marshal(header)
	if err != nil {
		f.Close()
		os.Remove(path)
		return nil, fmt.Errorf("recording: marshal header: %w", err)
	}

	if _, err := f.Write(append(headerJSON, '\n')); err != nil {
		f.Close()
		os.Remove(path)
		return nil, fmt.Errorf("recording: write header: %w", err)
	}

	return &Recorder{
		id:        id,
		file:      f,
		startTime: now,
		width:     width,
		height:    height,
	}, nil
}

// ID returns the unique recording identifier.
func (r *Recorder) ID() string {
	return r.id
}

// WriteOutput records an output frame.
func (r *Recorder) WriteOutput(data string) {
	r.writeFrame("o", data)
}

// WriteInput records an input frame.
func (r *Recorder) WriteInput(data string) {
	r.writeFrame("i", data)
}

// WriteResize records a terminal resize event.
func (r *Recorder) WriteResize(rows, cols int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}

	r.width = cols
	r.height = rows

	elapsed := time.Since(r.startTime).Seconds()
	data := fmt.Sprintf("%dx%d", cols, rows)
	frame := []any{elapsed, "r", data}

	frameJSON, err := json.Marshal(frame)
	if err != nil {
		return
	}
	r.file.Write(append(frameJSON, '\n'))
}

// Close finalizes the recording file.
func (r *Recorder) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	return r.file.Close()
}

func (r *Recorder) writeFrame(eventType, data string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}

	elapsed := time.Since(r.startTime).Seconds()
	frame := []any{elapsed, eventType, data}

	frameJSON, err := json.Marshal(frame)
	if err != nil {
		return
	}
	r.file.Write(append(frameJSON, '\n'))
}

func recordingsDir() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = "."
	}
	dir := filepath.Join(configDir, "mimir", "recordings")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("recording: create dir: %w", err)
	}
	return dir, nil
}
