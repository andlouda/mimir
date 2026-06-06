package transcript

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
)

// isolateConfigDir points os.UserConfigDir at a fresh temp directory on every
// supported OS. HOME / XDG_CONFIG_HOME cover Linux and macOS; APPDATA is
// what os.UserConfigDir reads on Windows. Without the APPDATA override every
// test ends up writing into the real %AppData%\mimir\transcripts\ and seeing
// each other's leftovers.
func isolateConfigDir(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("APPDATA", tmp)
}

func TestAppendAndReadTail(t *testing.T) {
	isolateConfigDir(t)

	path, err := Append("resume-test", "hello")
	if err != nil {
		t.Fatalf("append failed: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected transcript file to exist: %v", err)
	}

	if _, err := Append("resume-test", " world"); err != nil {
		t.Fatalf("second append failed: %v", err)
	}

	got, err := ReadTail("resume-test", 64)
	if err != nil {
		t.Fatalf("read tail failed: %v", err)
	}
	if got != "hello world" {
		t.Fatalf("unexpected transcript content: %q", got)
	}
}

func TestReadTailMissingFileReturnsEmpty(t *testing.T) {
	isolateConfigDir(t)

	got, err := ReadTail("missing", 128)
	if err != nil {
		t.Fatalf("read tail should not fail for missing file: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty transcript, got %q", got)
	}
}

func TestRejectsUnsafeResumeID(t *testing.T) {
	isolateConfigDir(t)

	if _, err := Append("../outside", "data"); err == nil {
		t.Fatalf("expected invalid resume id to be rejected")
	}
}

func TestReadFullReturnsCompleteTranscript(t *testing.T) {
	isolateConfigDir(t)

	want := "line one\nline two\nline three\n"
	if _, err := Append("resume-full", want); err != nil {
		t.Fatalf("append failed: %v", err)
	}

	got, err := ReadFull("resume-full", 0)
	if err != nil {
		t.Fatalf("read full failed: %v", err)
	}
	if got != want {
		t.Fatalf("expected full transcript, got %q", got)
	}
}

func TestListReturnsEntriesNewestFirst(t *testing.T) {
	isolateConfigDir(t)

	if _, err := Append("alpha", "first"); err != nil {
		t.Fatalf("seed alpha: %v", err)
	}
	if _, err := Append("beta", "second"); err != nil {
		t.Fatalf("seed beta: %v", err)
	}

	// Force a deterministic ordering by stamping mtimes; some filesystems
	// have second-resolution mtimes so two writes in the same test can land
	// in the same tick.
	dir, err := transcriptsDir()
	if err != nil {
		t.Fatalf("transcripts dir: %v", err)
	}
	now := time.Now()
	if err := os.Chtimes(filepath.Join(dir, "alpha.log"), now, now.Add(-time.Hour)); err != nil {
		t.Fatalf("chtimes alpha: %v", err)
	}
	if err := os.Chtimes(filepath.Join(dir, "beta.log"), now, now); err != nil {
		t.Fatalf("chtimes beta: %v", err)
	}

	// Drop a stray non-transcript file to confirm filtering.
	if err := os.WriteFile(filepath.Join(dir, "README"), []byte("ignore me"), 0o600); err != nil {
		t.Fatalf("write stray file: %v", err)
	}

	entries, err := List()
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d (%+v)", len(entries), entries)
	}
	if entries[0].ResumeID != "beta" {
		t.Fatalf("expected newest entry beta first, got %q", entries[0].ResumeID)
	}
	if entries[1].ResumeID != "alpha" {
		t.Fatalf("expected oldest entry alpha last, got %q", entries[1].ResumeID)
	}
	if entries[0].Size != int64(len("second")) {
		t.Fatalf("unexpected size for beta: %d", entries[0].Size)
	}
}

func TestListReturnsNothingForEmptyDir(t *testing.T) {
	isolateConfigDir(t)

	entries, err := List()
	if err != nil {
		t.Fatalf("list on empty dir should not error: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected empty list, got %d entries", len(entries))
	}
}

func TestMetadataPersistsAndIsListed(t *testing.T) {
	isolateConfigDir(t)

	if _, err := Append("api-prod-1", "boot"); err != nil {
		t.Fatalf("seed transcript: %v", err)
	}
	meta := Metadata{
		Name:         "API production",
		Type:         "ssh",
		SSHProfileID: "prod-api",
	}
	if err := WriteMetadata("api-prod-1", meta); err != nil {
		t.Fatalf("write metadata: %v", err)
	}

	got, err := ReadMetadata("api-prod-1")
	if err != nil {
		t.Fatalf("read metadata: %v", err)
	}
	if got.Name != "API production" || got.Type != "ssh" || got.SSHProfileID != "prod-api" {
		t.Fatalf("metadata round-trip lost fields: %+v", got)
	}
	if got.StartedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Fatalf("expected startedAt/updatedAt to be set, got %+v", got)
	}

	entries, err := List()
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected single entry, got %+v", entries)
	}
	if entries[0].Metadata.Name != "API production" {
		t.Fatalf("expected metadata name on list entry, got %+v", entries[0].Metadata)
	}
}

func TestWriteMetadataPreservesStartedAt(t *testing.T) {
	isolateConfigDir(t)

	if err := WriteMetadata("first", Metadata{Name: "first label"}); err != nil {
		t.Fatalf("first write: %v", err)
	}
	first, err := ReadMetadata("first")
	if err != nil {
		t.Fatalf("read first: %v", err)
	}

	// Ensure a measurable interval elapses so the UpdatedAt advances even
	// on filesystems with coarse timestamp resolution.
	time.Sleep(10 * time.Millisecond)

	if err := WriteMetadata("first", Metadata{Name: "renamed"}); err != nil {
		t.Fatalf("second write: %v", err)
	}
	second, err := ReadMetadata("first")
	if err != nil {
		t.Fatalf("read second: %v", err)
	}
	if !second.StartedAt.Equal(first.StartedAt) {
		t.Fatalf("startedAt should be preserved across renames: first=%v second=%v", first.StartedAt, second.StartedAt)
	}
	if !second.UpdatedAt.After(first.UpdatedAt) {
		t.Fatalf("updatedAt should advance on rename: first=%v second=%v", first.UpdatedAt, second.UpdatedAt)
	}
	if second.Name != "renamed" {
		t.Fatalf("expected new name to win, got %q", second.Name)
	}
}

func TestWriteMetadataIsNoopWhenContentUnchanged(t *testing.T) {
	isolateConfigDir(t)

	original := Metadata{Name: "stable", Type: "ssh", SSHProfileID: "prod"}
	if err := WriteMetadata("noop", original); err != nil {
		t.Fatalf("first write: %v", err)
	}
	path, err := metadataPath("noop")
	if err != nil {
		t.Fatalf("path: %v", err)
	}
	info1, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	// Sleep enough that any rewrite would land with a newer mtime even on
	// coarse-resolution filesystems.
	time.Sleep(20 * time.Millisecond)

	if err := WriteMetadata("noop", original); err != nil {
		t.Fatalf("second write: %v", err)
	}
	info2, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat 2: %v", err)
	}
	if !info2.ModTime().Equal(info1.ModTime()) {
		t.Fatalf("expected no rewrite when metadata unchanged: %v -> %v", info1.ModTime(), info2.ModTime())
	}

	// Now actually change something — the file must update.
	changed := original
	changed.Name = "renamed"
	if err := WriteMetadata("noop", changed); err != nil {
		t.Fatalf("third write: %v", err)
	}
	info3, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat 3: %v", err)
	}
	if !info3.ModTime().After(info1.ModTime()) {
		t.Fatalf("expected rewrite when name changed: %v -> %v", info1.ModTime(), info3.ModTime())
	}
}

func TestReadContentReportsTruncationAuthoritatively(t *testing.T) {
	isolateConfigDir(t)

	// Write 1000 bytes; ask for 200.
	full := strings.Repeat("x", 1000)
	if _, err := Append("trunc", full); err != nil {
		t.Fatalf("seed: %v", err)
	}
	content, err := ReadContent("trunc", 200)
	if err != nil {
		t.Fatalf("read content: %v", err)
	}
	if !content.Truncated {
		t.Fatalf("expected Truncated=true when maxBytes < size: %+v", content)
	}
	if content.Size != 1000 {
		t.Fatalf("expected Size=1000, got %d", content.Size)
	}
	if content.ReadBytes != int64(len(content.Text)) {
		t.Fatalf("ReadBytes (%d) and len(Text) (%d) must match", content.ReadBytes, len(content.Text))
	}
	if content.ReadBytes > 200 {
		t.Fatalf("ReadBytes %d exceeds requested cap 200", content.ReadBytes)
	}
}

func TestReadContentFullFileReportsNotTruncated(t *testing.T) {
	isolateConfigDir(t)

	if _, err := Append("complete", "small file content"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	content, err := ReadContent("complete", 1024)
	if err != nil {
		t.Fatalf("read content: %v", err)
	}
	if content.Truncated {
		t.Fatalf("expected Truncated=false for cap above file size: %+v", content)
	}
	if content.Text != "small file content" {
		t.Fatalf("unexpected text: %q", content.Text)
	}
	if content.Size != int64(len("small file content")) {
		t.Fatalf("size mismatch: %d", content.Size)
	}
}

func TestReadContentMissingFileReturnsEmpty(t *testing.T) {
	isolateConfigDir(t)

	content, err := ReadContent("never-existed", 1024)
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if content.Text != "" || content.Size != 0 || content.Truncated {
		t.Fatalf("expected zero content for missing file: %+v", content)
	}
}

func TestReadTailDoesNotSplitUTF8(t *testing.T) {
	isolateConfigDir(t)

	// "ä" is two bytes in UTF-8 (0xC3 0xA4). A byte-naive tail-read whose
	// cut falls between those two bytes would return invalid UTF-8.
	// Construct content so the chosen cap lands inside such a sequence.
	prefix := strings.Repeat("x", 10)
	mid := "ää" // 4 bytes, two runes
	suffix := strings.Repeat("y", 10)
	if _, err := Append("utf8", prefix+mid+suffix); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Total bytes = 10 + 4 + 10 = 24. Ask for last 13 bytes — that cut lands
	// inside the second "ä" (24-13=11 = 10 prefix + 1 byte into mid).
	got, err := ReadTail("utf8", 13)
	if err != nil {
		t.Fatalf("read tail: %v", err)
	}
	if !utf8.ValidString(got) {
		t.Fatalf("ReadTail returned invalid UTF-8: %q", got)
	}
	// And the returned content must not exceed the cap.
	if len(got) > 13 {
		t.Fatalf("ReadTail returned %d bytes, cap was 13", len(got))
	}
}

func TestReadTailReadsOnlyTheTailNotTheWholeFile(t *testing.T) {
	isolateConfigDir(t)

	// Write a 200 KiB transcript and ask for only the last 1 KiB.
	const total = 200 * 1024
	const want = 1024
	big := make([]byte, total)
	for i := range big {
		big[i] = 'a' + byte(i%26)
	}
	if _, err := Append("big", string(big)); err != nil {
		t.Fatalf("seed: %v", err)
	}

	got, err := ReadTail("big", want)
	if err != nil {
		t.Fatalf("read tail: %v", err)
	}
	if len(got) != want {
		t.Fatalf("expected exactly %d bytes back, got %d", want, len(got))
	}
	// And it really must be the *tail*, not the head.
	if got != string(big[total-want:]) {
		t.Fatalf("tail mismatch — expected last %d bytes, got something else", want)
	}
}

func TestWriteMetadataIsAtomic(t *testing.T) {
	isolateConfigDir(t)

	if err := WriteMetadata("atomic", Metadata{Name: "first", Type: "ssh"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := ReadMetadata("atomic")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got.Name != "first" {
		t.Fatalf("roundtrip lost name: %+v", got)
	}

	// Atomic-write contract: no leftover temp files in the target directory
	// after a successful write. safeio.AtomicWriteFile creates ".tmp-*" files
	// during the write and renames them into place; if any are left over the
	// next list/scan picks them up as junk.
	dir, err := transcriptsDir()
	if err != nil {
		t.Fatalf("dir: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tmp-") {
			t.Fatalf("unexpected leftover temp file %s — atomic write did not clean up", e.Name())
		}
	}
}

func TestAppendIsSafeUnderParallelLoad(t *testing.T) {
	isolateConfigDir(t)

	// Sanity-check that parallel goroutines appending to the *same* resume ID
	// produce a byte-exact transcript: each fixed-size record lands intact,
	// and the total byte count matches the expected sum. Without a per-resume
	// mutex, Append on Windows could interleave writes (POSIX is somewhat
	// safer for sub-PIPE_BUF writes but not guaranteed for larger ones).
	const (
		goroutines    = 16
		writesPerLine = 200
		recordSize    = 64
	)

	record := strings.Repeat("a", recordSize-1) + "\n"

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < writesPerLine; i++ {
				if _, err := Append("parallel", record); err != nil {
					t.Errorf("append: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()

	got, err := ReadFull("parallel", 0)
	if err != nil {
		t.Fatalf("read full: %v", err)
	}
	expected := goroutines * writesPerLine * recordSize
	if len(got) != expected {
		t.Fatalf("expected %d bytes after parallel appends, got %d", expected, len(got))
	}
	// Every line should be the fixed-size record. A torn write would leave
	// a line shorter than recordSize or with the trailing newline lost.
	lines := strings.Split(got, "\n")
	// Trailing empty after last \n.
	for i, line := range lines[:len(lines)-1] {
		if len(line) != recordSize-1 {
			t.Fatalf("line %d has unexpected length %d (record corruption): %q", i, len(line), line)
		}
	}
}

func TestReadMetadataMissingReturnsZero(t *testing.T) {
	isolateConfigDir(t)

	meta, err := ReadMetadata("never-written")
	if err != nil {
		t.Fatalf("read missing metadata should not error: %v", err)
	}
	if meta.Name != "" || !meta.StartedAt.IsZero() {
		t.Fatalf("expected zero metadata, got %+v", meta)
	}
}
