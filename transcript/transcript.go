package transcript

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

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
