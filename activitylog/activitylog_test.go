package activitylog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupTestDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))
}

func TestAppendAndRead(t *testing.T) {
	setupTestDir(t)

	entry := SecurityEventEntry{
		Timestamp: "2026-01-01T00:00:00Z",
		Event:     "test_event",
		Operation: "test_op",
	}
	if err := Append(KindSecurityEvents, entry); err != nil {
		t.Fatalf("Append: %v", err)
	}

	entries, err := Read(KindSecurityEvents, 0)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("len = %d, want 1", len(entries))
	}
	if entries[0]["event"] != "test_event" {
		t.Fatalf("event = %v", entries[0]["event"])
	}
}

func TestReadEmptyLog(t *testing.T) {
	setupTestDir(t)

	entries, err := Read(KindSecurityEvents, 0)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected empty, got %d", len(entries))
	}
}

func TestReadWithLimit(t *testing.T) {
	setupTestDir(t)

	for i := 0; i < 5; i++ {
		_ = Append(KindToolExecutions, ToolExecutionEntry{
			Timestamp: "2026-01-01T00:00:00Z",
			ToolID:    "tool",
			ToolName:  "test",
		})
	}

	entries, err := Read(KindToolExecutions, 3)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("len = %d, want 3", len(entries))
	}
}

func TestUnknownKind(t *testing.T) {
	setupTestDir(t)

	if err := Append("unknown_kind", struct{}{}); err == nil {
		t.Fatal("expected error for unknown kind")
	}

	_, err := Read("unknown_kind", 0)
	if err == nil {
		t.Fatal("expected error for unknown kind")
	}
}

func TestRedactAPIKey(t *testing.T) {
	entry := map[string]any{
		"api_key": "secret-value",
		"message": "hello",
	}

	payload, err := marshalRedacted(entry)
	if err != nil {
		t.Fatalf("marshalRedacted: %v", err)
	}

	var decoded map[string]any
	_ = json.Unmarshal(payload, &decoded)

	if decoded["api_key"] != "[REDACTED]" {
		t.Fatalf("api_key = %v, want [REDACTED]", decoded["api_key"])
	}
	if decoded["message"] != "hello" {
		t.Fatalf("message = %v, want hello", decoded["message"])
	}
}

func TestRedactBearerToken(t *testing.T) {
	entry := map[string]any{
		"output": "Authorization: Bearer sk-abc123456789012345",
	}

	payload, _ := marshalRedacted(entry)
	var decoded map[string]any
	_ = json.Unmarshal(payload, &decoded)

	output := decoded["output"].(string)
	if strings.Contains(output, "sk-abc") {
		t.Fatalf("bearer token not redacted: %s", output)
	}
}

func TestRedactNestedKeys(t *testing.T) {
	entry := map[string]any{
		"metadata": map[string]any{
			"password": "hunter2",
			"host":     "example.com",
		},
	}

	payload, _ := marshalRedacted(entry)
	var decoded map[string]any
	_ = json.Unmarshal(payload, &decoded)

	meta := decoded["metadata"].(map[string]any)
	if meta["password"] != "[REDACTED]" {
		t.Fatalf("password = %v, want [REDACTED]", meta["password"])
	}
	if meta["host"] != "example.com" {
		t.Fatalf("host = %v, want example.com", meta["host"])
	}
}

func TestKinds(t *testing.T) {
	kinds := Kinds()
	if len(kinds) != 5 {
		t.Fatalf("len = %d, want 5", len(kinds))
	}
	for i := 1; i < len(kinds); i++ {
		if kinds[i] < kinds[i-1] {
			t.Fatalf("not sorted: %v", kinds)
		}
	}
}

func TestRotation(t *testing.T) {
	setupTestDir(t)

	logPath, err := getLogPath(KindSecurityEvents)
	if err != nil {
		t.Fatal(err)
	}

	big := strings.Repeat("x", int(maxLogFileBytes)+1)
	if err := os.MkdirAll(filepath.Dir(logPath), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logPath, []byte(big), 0600); err != nil {
		t.Fatal(err)
	}

	_ = Append(KindSecurityEvents, SecurityEventEntry{
		Timestamp: "2026-01-01T00:00:00Z",
		Event:     "after_rotation",
	})

	if _, err := os.Stat(logPath + ".1"); err != nil {
		t.Fatal("rotated file should exist")
	}

	entries, _ := Read(KindSecurityEvents, 0)
	if len(entries) != 1 || entries[0]["event"] != "after_rotation" {
		t.Fatalf("entries = %+v", entries)
	}
}
