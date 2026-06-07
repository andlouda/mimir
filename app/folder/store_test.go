package folder

import (
	"os"
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *FolderStore {
	t.Helper()
	dir := t.TempDir()
	s := &FolderStore{
		filePath: filepath.Join(dir, "folders.json"),
		folders:  []Folder{},
	}
	return s
}

func TestCreateAndList(t *testing.T) {
	s := newTestStore(t)

	folders, err := s.Create(Folder{Name: "Dev"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(folders) != 1 {
		t.Fatalf("len = %d, want 1", len(folders))
	}
	if folders[0].Name != "Dev" {
		t.Fatalf("name = %q", folders[0].Name)
	}
	if folders[0].ID == "" {
		t.Fatal("ID should be set")
	}

	list := s.List()
	if len(list) != 1 || list[0].Name != "Dev" {
		t.Fatalf("List = %+v", list)
	}
}

func TestUpdate(t *testing.T) {
	s := newTestStore(t)
	folders, _ := s.Create(Folder{Name: "Old"})
	id := folders[0].ID

	updated, err := s.Update(Folder{ID: id, Name: "New", Position: 5})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated[0].Name != "New" || updated[0].Position != 5 {
		t.Fatalf("updated = %+v", updated[0])
	}
}

func TestUpdateNotFound(t *testing.T) {
	s := newTestStore(t)

	_, err := s.Update(Folder{ID: "nonexistent", Name: "x"})
	if err == nil {
		t.Fatal("expected error for missing folder")
	}
}

func TestDelete(t *testing.T) {
	s := newTestStore(t)
	folders, _ := s.Create(Folder{Name: "A"})
	folders, _ = s.Create(Folder{Name: "B"})
	idA := folders[0].ID

	remaining, err := s.Delete(idA)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(remaining) != 1 {
		t.Fatalf("len = %d, want 1", len(remaining))
	}
	if remaining[0].Name != "B" {
		t.Fatalf("remaining = %+v", remaining[0])
	}
}

func TestPersistence(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "folders.json")

	s1 := &FolderStore{filePath: fp, folders: []Folder{}}
	_, _ = s1.Create(Folder{Name: "Persist"})

	s2 := &FolderStore{filePath: fp}
	if err := s2.load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(s2.folders) != 1 || s2.folders[0].Name != "Persist" {
		t.Fatalf("loaded = %+v", s2.folders)
	}
}

func TestLoadMissingFile(t *testing.T) {
	s := &FolderStore{filePath: filepath.Join(t.TempDir(), "missing.json")}
	if err := s.load(); err != nil {
		t.Fatalf("load missing: %v", err)
	}
	if len(s.folders) != 0 {
		t.Fatalf("expected empty, got %d", len(s.folders))
	}
}

func TestListReturnsDefensiveCopy(t *testing.T) {
	s := newTestStore(t)
	_, _ = s.Create(Folder{Name: "X"})

	list := s.List()
	list[0].Name = "mutated"

	fresh := s.List()
	if fresh[0].Name != "X" {
		t.Fatal("List should return a defensive copy")
	}
}

func TestCreateSetsPosition(t *testing.T) {
	s := newTestStore(t)
	folders, _ := s.Create(Folder{Name: "First"})
	if folders[0].Position != 1 {
		t.Fatalf("position = %d, want 1", folders[0].Position)
	}

	folders, _ = s.Create(Folder{Name: "Second"})
	if folders[1].Position != 2 {
		t.Fatalf("position = %d, want 2", folders[1].Position)
	}
}

func TestDeletePersists(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "folders.json")

	s := &FolderStore{filePath: fp, folders: []Folder{}}
	folders, _ := s.Create(Folder{Name: "Gone"})
	id := folders[0].ID
	_, _ = s.Delete(id)

	data, err := os.ReadFile(fp)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "[]" {
		t.Fatalf("file = %s, want []", data)
	}
}
