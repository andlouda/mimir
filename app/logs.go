package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"mimir/activitylog"
)

type ActivityLogEntry struct {
	Kind      string         `json:"kind"`
	Timestamp string         `json:"timestamp"`
	Title     string         `json:"title"`
	Summary   string         `json:"summary"`
	Raw       map[string]any `json:"raw"`
}

func (a *App) GetActivityLogKindsJSON() (string, error) {
	payload, err := json.Marshal(activitylog.Kinds())
	if err != nil {
		return "", fmt.Errorf("failed to encode activity log kinds: %w", err)
	}
	return string(payload), nil
}

func (a *App) GetActivityLogsJSON(kind string, limit int) (string, error) {
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}

	kinds := activitylog.Kinds()
	if kind != "" && kind != "all" {
		kinds = []string{kind}
	}

	entries := make([]ActivityLogEntry, 0)
	for _, logKind := range kinds {
		rawEntries, err := activitylog.Read(logKind, limit)
		if err != nil {
			return "", err
		}
		for _, raw := range rawEntries {
			entries = append(entries, normalizeActivityLogEntry(logKind, raw))
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		left := parseLogTimestamp(entries[i].Timestamp)
		right := parseLogTimestamp(entries[j].Timestamp)
		if left.Equal(right) {
			return entries[i].Title > entries[j].Title
		}
		return left.After(right)
	})

	if len(entries) > limit {
		entries = entries[:limit]
	}

	payload, err := json.Marshal(entries)
	if err != nil {
		return "", fmt.Errorf("failed to encode activity logs: %w", err)
	}
	return string(payload), nil
}

func normalizeActivityLogEntry(kind string, raw map[string]any) ActivityLogEntry {
	title, summary := buildActivityLogText(kind, raw)
	return ActivityLogEntry{
		Kind:      kind,
		Timestamp: stringValue(raw["timestamp"]),
		Title:     title,
		Summary:   summary,
		Raw:       raw,
	}
}

func buildActivityLogText(kind string, raw map[string]any) (string, string) {
	switch kind {
	case activitylog.KindAIInteractions:
		mode := stringValue(raw["mode"])
		provider := stringValue(raw["provider"])
		model := stringValue(raw["model"])
		title := "AI Interaction"
		if mode != "" {
			title = "AI: " + prettifyToken(mode)
		}
		summary := joinNonEmpty(
			firstNonEmpty(stringValue(raw["goal"]), stringValue(raw["prompt"])),
			joinNonEmpty(provider, model),
			stringValue(raw["error"]),
		)
		return title, summary
	case activitylog.KindWorkflowRuns:
		event := prettifyToken(stringValue(raw["event"]))
		title := "Workflow"
		if event != "" {
			title = "Workflow: " + event
		}
		summary := joinNonEmpty(stringValue(raw["workflowId"]), stringValue(raw["stepId"]), stringValue(raw["message"]))
		return title, summary
	case activitylog.KindToolExecutions:
		title := "Tool Execution"
		if toolName := stringValue(raw["toolName"]); toolName != "" {
			title = "Tool: " + toolName
		}
		summary := joinNonEmpty(
			stringValue(raw["source"]),
			stringValue(raw["workflowId"]),
			stringValue(raw["stepId"]),
			stringValue(raw["error"]),
			stringValue(raw["output"]),
		)
		return title, summary
	case activitylog.KindSecurityEvents:
		event := prettifyToken(stringValue(raw["event"]))
		title := "Security Event"
		if event != "" {
			title = "Security: " + event
		}
		summary := joinNonEmpty(
			stringValue(raw["operation"]),
			stringValue(raw["path"]),
			stringValue(raw["reason"]),
		)
		return title, summary
	case activitylog.KindApprovalEvents:
		event := prettifyToken(stringValue(raw["event"]))
		title := "Approval Event"
		if event != "" {
			title = "Approval: " + event
		}
		summary := joinNonEmpty(
			stringValue(raw["toolName"]),
			stringValue(raw["workflowId"]),
			stringValue(raw["risk"]),
			stringValue(raw["reason"]),
		)
		return title, summary
	default:
		return kind, ""
	}
}

func prettifyToken(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}
	input = strings.ReplaceAll(input, "_", " ")
	parts := strings.Fields(input)
	for i, part := range parts {
		if part == "" {
			continue
		}
		runes := []rune(strings.ToLower(part))
		runes[0] = []rune(strings.ToUpper(string(runes[0])))[0]
		parts[i] = string(runes)
	}
	return strings.Join(parts, " ")
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case bool:
		if typed {
			return "true"
		}
		return "false"
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func joinNonEmpty(values ...string) string {
	filtered := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			filtered = append(filtered, value)
		}
	}
	return strings.Join(filtered, " · ")
}

func parseLogTimestamp(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}
