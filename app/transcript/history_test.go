package transcript

import (
	"os"
	"strings"
	"testing"
)

func TestHistoryRoundTripAndReplace(t *testing.T) {
	isolateConfigDir(t)

	if got, err := ReadHistory("pane1", 0); err != nil || got.Text != "" || got.Size != 0 {
		t.Fatalf("missing snapshot: %+v, %v", got, err)
	}
	if err := WriteHistory("pane1", "first line\nsecond line\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	// A snapshot replaces the previous one as a whole.
	if err := WriteHistory("pane1", "one very long logical line that no pane width has cut\n"); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	got, err := ReadHistory("pane1", 0)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got.Text != "one very long logical line that no pane width has cut\n" || got.Truncated {
		t.Fatalf("snapshot: %+v", got)
	}
}

func TestWriteHistoryKeepsSnapshotWhenSessionIsGone(t *testing.T) {
	isolateConfigDir(t)

	if err := WriteHistory("pane1", "kept\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	for _, empty := range []string{"", "\n", "   \n\n"} {
		if err := WriteHistory("pane1", empty); err != nil {
			t.Fatalf("empty write: %v", err)
		}
	}
	if got, _ := ReadHistory("pane1", 0); got.Text != "kept\n" {
		t.Fatalf("empty capture wiped the snapshot: %q", got.Text)
	}
}

func TestReadHistoryTailStartsAtALine(t *testing.T) {
	isolateConfigDir(t)

	text := strings.Repeat("0123456789 äöü\n", 100)
	if err := WriteHistory("pane1", text); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := ReadHistory("pane1", 250)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !got.Truncated || got.Size != int64(len(text)) || got.ReadBytes != int64(len(got.Text)) {
		t.Fatalf("tail metadata: %+v", got)
	}
	if !strings.HasPrefix(got.Text, "0123456789 äöü\n") || !strings.HasSuffix(got.Text, "äöü\n") {
		t.Fatalf("tail does not start at a line: %q", got.Text[:20])
	}
}

func TestHistoryRejectsBadIDsAndIsDeletedWithTheTranscript(t *testing.T) {
	isolateConfigDir(t)

	if err := WriteHistory("../escape", "x\n"); err == nil {
		t.Fatal("path traversal id accepted")
	}
	if _, err := ReadHistory("../escape", 0); err == nil {
		t.Fatal("path traversal id accepted on read")
	}

	if _, err := Append("pane1", "raw stream"); err != nil {
		t.Fatalf("append: %v", err)
	}
	if err := WriteHistory("pane1", "snapshot\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	path, _ := historyPath("pane1")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("snapshot file missing: %v", err)
	}
	if err := Delete("pane1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("snapshot survived the delete: %v", err)
	}
	// The snapshot is not listed as a transcript of its own.
	if err := WriteHistory("pane2", "only a snapshot\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	entries, err := List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, e := range entries {
		if strings.Contains(e.ResumeID, "history") {
			t.Fatalf("snapshot listed as a transcript: %+v", e)
		}
	}
}
