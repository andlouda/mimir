package activitylog

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
)

const (
	KindAIInteractions = "ai_interactions"
	KindWorkflowRuns   = "workflow_runs"
	KindToolExecutions = "tool_executions"
	KindSecurityEvents = "security_events"
	KindApprovalEvents = "approval_events"
)

var (
	logMu sync.Mutex
	files = map[string]string{
		KindAIInteractions: "ai_interactions.jsonl",
		KindWorkflowRuns:   "workflow_runs.jsonl",
		KindToolExecutions: "tool_executions.jsonl",
		KindSecurityEvents: "security_events.jsonl",
		KindApprovalEvents: "approval_events.jsonl",
	}
	sensitiveKeyPattern   = regexp.MustCompile(`(?i)(api[_-]?key|authorization|bearer|cookie|pass(word)?|secret|token|private[_-]?key|refresh[_-]?token)`)
	bearerValuePattern    = regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._\-=/+]{10,}`)
	openAIKeyValuePattern = regexp.MustCompile(`\bsk-[A-Za-z0-9\-_]{12,}\b`)
)

const maxLogFileBytes int64 = 10 * 1024 * 1024

type WorkflowRunEntry struct {
	Timestamp  string            `json:"timestamp"`
	WorkflowID string            `json:"workflowId"`
	Event      string            `json:"event"`
	StepID     string            `json:"stepId,omitempty"`
	Message    string            `json:"message,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

type ToolExecutionEntry struct {
	Timestamp    string            `json:"timestamp"`
	Source       string            `json:"source"`
	WorkflowID   string            `json:"workflowId,omitempty"`
	StepID       string            `json:"stepId,omitempty"`
	ToolID       string            `json:"toolId"`
	ToolName     string            `json:"toolName"`
	TerminalID   int               `json:"terminalId,omitempty"`
	TerminalType string            `json:"terminalType,omitempty"`
	Inputs       map[string]string `json:"inputs,omitempty"`
	Output       string            `json:"output,omitempty"`
	Error        string            `json:"error,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

type SecurityEventEntry struct {
	Timestamp string            `json:"timestamp"`
	Event     string            `json:"event"`
	Operation string            `json:"operation"`
	Path      string            `json:"path,omitempty"`
	Reason    string            `json:"reason,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

type ApprovalEventEntry struct {
	Timestamp  string            `json:"timestamp"`
	WorkflowID string            `json:"workflowId,omitempty"`
	StepID     string            `json:"stepId"`
	ToolID     string            `json:"toolId"`
	ToolName   string            `json:"toolName"`
	Risk       string            `json:"risk"`
	Event      string            `json:"event"`
	Reason     string            `json:"reason,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

func getLogPath(kind string) (string, error) {
	filename, ok := files[kind]
	if !ok {
		return "", fmt.Errorf("unknown log kind: %s", kind)
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user config directory: %w", err)
	}

	appConfigDir := filepath.Join(configDir, "mimir")
	if err := os.MkdirAll(appConfigDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create application config directory: %w", err)
	}

	return filepath.Join(appConfigDir, filename), nil
}

func Append(kind string, entry any) error {
	logPath, err := getLogPath(kind)
	if err != nil {
		return err
	}

	payload, err := marshalRedacted(entry)
	if err != nil {
		return fmt.Errorf("failed to encode log entry: %w", err)
	}

	logMu.Lock()
	defer logMu.Unlock()

	if err := rotateIfNeeded(logPath); err != nil {
		return err
	}

	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer file.Close()

	if _, err := file.Write(append(payload, '\n')); err != nil {
		return fmt.Errorf("failed to append log entry: %w", err)
	}

	return nil
}

func marshalRedacted(entry any) ([]byte, error) {
	payload, err := json.Marshal(entry)
	if err != nil {
		return nil, err
	}

	var decoded any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return nil, err
	}

	return json.Marshal(redactAny(decoded, ""))
}

func redactAny(value any, key string) any {
	if sensitiveKeyPattern.MatchString(key) {
		return "[REDACTED]"
	}

	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for childKey, childValue := range typed {
			out[childKey] = redactAny(childValue, childKey)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, childValue := range typed {
			out[i] = redactAny(childValue, key)
		}
		return out
	case string:
		redacted := bearerValuePattern.ReplaceAllString(typed, "Bearer [REDACTED]")
		redacted = openAIKeyValuePattern.ReplaceAllString(redacted, "[REDACTED]")
		return redacted
	default:
		return value
	}
}

func rotateIfNeeded(logPath string) error {
	info, err := os.Stat(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to stat log file: %w", err)
	}
	if info.Size() < maxLogFileBytes {
		return nil
	}

	rotatedPath := logPath + ".1"
	_ = os.Remove(rotatedPath)
	if err := os.Rename(logPath, rotatedPath); err != nil {
		return fmt.Errorf("failed to rotate log file: %w", err)
	}
	return nil
}

func Kinds() []string {
	kinds := make([]string, 0, len(files))
	for kind := range files {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	return kinds
}

func Read(kind string, limit int) ([]map[string]any, error) {
	logPath, err := getLogPath(kind)
	if err != nil {
		return nil, err
	}

	file, err := os.Open(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []map[string]any{}, nil
		}
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	maxCapacity := 1024 * 1024
	scanner.Buffer(make([]byte, 0, 64*1024), maxCapacity)

	entries := make([]map[string]any, 0)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry map[string]any
		if err := json.Unmarshal(line, &entry); err != nil {
			entry = map[string]any{
				"timestamp": "",
				"error":     "failed to parse log entry",
				"raw":       string(line),
			}
		}
		entries = append(entries, entry)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read log file: %w", err)
	}

	if limit > 0 && len(entries) > limit {
		entries = entries[len(entries)-limit:]
	}

	return entries, nil
}
