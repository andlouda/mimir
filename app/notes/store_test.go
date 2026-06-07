package notes

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestStore(t *testing.T) *NoteStore {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "notes")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	return &NoteStore{baseDir: dir}
}

func TestSaveAndGet(t *testing.T) {
	s := newTestStore(t)

	if err := s.Save("test.md", "# Hello\nworld"); err != nil {
		t.Fatalf("Save: %v", err)
	}

	note, err := s.Get("test.md")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if note.Content != "# Hello\nworld" {
		t.Fatalf("content = %q", note.Content)
	}
	if note.Filename != "test.md" {
		t.Fatalf("filename = %q", note.Filename)
	}
}

func TestList(t *testing.T) {
	s := newTestStore(t)
	_ = s.Save("a.md", "alpha")
	_ = s.Save("b.md", "# Beta Title\nbody")

	notes, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(notes) != 2 {
		t.Fatalf("len = %d, want 2", len(notes))
	}

	titles := map[string]bool{}
	for _, n := range notes {
		titles[n.Title] = true
	}
	if !titles["a"] {
		t.Fatal("missing title 'a'")
	}
	if !titles["Beta Title"] {
		t.Fatal("missing title 'Beta Title'")
	}
}

func TestDelete(t *testing.T) {
	s := newTestStore(t)
	_ = s.Save("del.md", "gone")

	if err := s.Delete("del.md"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := s.Get("del.md")
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestRename(t *testing.T) {
	s := newTestStore(t)
	_ = s.Save("old.md", "content")

	if err := s.Rename("old.md", "new.md"); err != nil {
		t.Fatalf("Rename: %v", err)
	}

	_, err := s.Get("old.md")
	if err == nil {
		t.Fatal("old name should not exist")
	}

	note, err := s.Get("new.md")
	if err != nil {
		t.Fatalf("Get new: %v", err)
	}
	if note.Content != "content" {
		t.Fatalf("content = %q", note.Content)
	}
}

func TestRenameConflict(t *testing.T) {
	s := newTestStore(t)
	_ = s.Save("a.md", "aaa")
	_ = s.Save("b.md", "bbb")

	err := s.Rename("a.md", "b.md")
	if err == nil {
		t.Fatal("expected error for rename conflict")
	}
}

func TestInvalidFilename(t *testing.T) {
	s := newTestStore(t)

	for _, name := range []string{"../evil.md", "foo.txt", "a/b.md", ".md", ""} {
		if err := s.Save(name, "x"); err == nil {
			t.Fatalf("expected error for filename %q", name)
		}
	}
}

func TestPathTraversal(t *testing.T) {
	s := newTestStore(t)

	err := s.Save("..%2f..%2fetc%2fpasswd.md", "x")
	if err == nil {
		t.Fatal("expected error for path traversal filename")
	}
}

func TestSizeLimitSave(t *testing.T) {
	s := newTestStore(t)

	big := strings.Repeat("x", maxNoteSize+1)
	if err := s.Save("big.md", big); err == nil {
		t.Fatal("expected error for oversized content")
	}
}

func TestSizeLimitGet(t *testing.T) {
	s := newTestStore(t)
	big := strings.Repeat("x", maxNoteSize+1)
	if err := os.WriteFile(filepath.Join(s.baseDir, "huge.md"), []byte(big), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := s.Get("huge.md")
	if err == nil {
		t.Fatal("expected error for oversized file on read")
	}
}

func TestExtractTitle(t *testing.T) {
	tests := []struct {
		content  string
		filename string
		want     string
	}{
		{"# My Title\nbody", "file.md", "My Title"},
		{"no heading here", "fallback.md", "fallback"},
		{"", "empty.md", "empty"},
		{"\n\n# Late Title", "late.md", "Late Title"},
	}
	for _, tt := range tests {
		got := extractTitle(tt.content, tt.filename)
		if got != tt.want {
			t.Errorf("extractTitle(%q, %q) = %q, want %q", tt.content, tt.filename, got, tt.want)
		}
	}
}
