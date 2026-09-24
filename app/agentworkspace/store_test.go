package agentworkspace

import (
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
)

func setup(t *testing.T) (*Store, Data) {
	t.Helper()
	s := NewStore(filepath.Join(t.TempDir(), "workspace.json"))
	d, err := s.SaveProject(Project{Name: "Mimir", Host: "local", Root: "/repo"})
	if err != nil {
		t.Fatal(err)
	}
	d, err = s.SaveTask(Task{ProjectID: d.Projects[0].ID, Title: "Fix SSH", Status: "in_progress"})
	if err != nil {
		t.Fatal(err)
	}
	return s, d
}

func TestAssignmentsSurviveReopenAndArchives(t *testing.T) {
	s, d := setup(t)
	ref := Session{ProjectID: d.Projects[0].ID, TaskID: d.Tasks[0].ID, Host: "local", Kind: "claude", File: "/sessions/one.jsonl", Title: "Fix SSH"}
	d, err := s.LinkSession(ref)
	if err != nil {
		t.Fatal(err)
	}
	back, err := NewStore(s.path).Snapshot()
	if err != nil || !reflect.DeepEqual(back, d) {
		t.Fatalf("restart: %v, %+v", err, back)
	}
	// Same path on a different host is a separate session.
	ref.Host = "server"
	d, err = s.LinkSession(ref)
	if err != nil || len(d.Sessions) != 2 {
		t.Fatalf("host isolation: %v", err)
	}
	// Idempotent assignment; no duplicate references.
	d, err = s.LinkSession(ref)
	if err != nil || len(d.Sessions) != 2 {
		t.Fatalf("idempotency: %v", err)
	}
	p := d.Projects[0]
	p.Archived = true
	d, err = s.SaveProject(p)
	if err != nil || len(d.Tasks) != 1 || len(d.Sessions) != 2 {
		t.Fatalf("archive destroyed records: %v", err)
	}
	if d.Tasks[0].Status != "in_progress" {
		t.Fatal("archiving changed task status")
	}
	p = d.Projects[0]
	p.Archived = false
	d, err = s.SaveProject(p)
	if err != nil {
		t.Fatal(err)
	}
	d, err = s.UnlinkSession(d.Sessions[0].ID)
	if err != nil || len(d.Sessions) != 1 || len(d.Tasks) != 1 {
		t.Fatalf("unlink: %v", err)
	}
}

func TestInvalidReferencesAndStaleEditsAreRejected(t *testing.T) {
	s, d := setup(t)
	ref := Session{ProjectID: d.Projects[0].ID, TaskID: "missing", Host: "local", Kind: "claude", File: "/sessions/one"}
	if _, err := s.LinkSession(ref); err == nil {
		t.Fatal("missing task accepted")
	}
	d, err := s.SaveProject(Project{Name: "Another"})
	if err != nil {
		t.Fatal(err)
	}
	ref.TaskID, ref.ProjectID = d.Tasks[0].ID, d.Projects[1].ID
	if _, err := s.LinkSession(ref); err == nil {
		t.Fatal("cross-project task accepted")
	}
	ref.ProjectID = d.Projects[0].ID
	if _, err := s.LinkSession(ref); err != nil {
		t.Fatal(err)
	}
	ref.TaskID = ""
	if _, err := s.LinkSession(ref); err == nil {
		t.Fatal("silently reassigned a session")
	}
	task := d.Tasks[0]
	task.Status = "done"
	if _, err := s.SaveTask(task); err != nil {
		t.Fatal(err)
	}
	task.Status = "planned"
	if _, err := s.SaveTask(task); err == nil {
		t.Fatal("stale update accepted")
	}
	if _, err := s.SaveProject(Project{Name: "bad\x1bname"}); err == nil {
		t.Fatal("control characters accepted")
	}
	if _, err := s.SaveTask(Task{ProjectID: ref.ProjectID, Title: "x", Status: "idle"}); err == nil {
		t.Fatal("agent state accepted as task status")
	}
	if _, err := s.SaveTask(Task{ProjectID: "missing", Title: "x", Status: "planned"}); err == nil {
		t.Fatal("orphan task accepted")
	}
}

func TestBrokenOrNewerFileIsPreserved(t *testing.T) {
	for _, raw := range []string{"{broken", "null", "{}", `{"version":2}`} {
		t.Run(raw, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "workspace.json")
			if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
				t.Fatal(err)
			}
			s := NewStore(path)
			if _, err := s.Snapshot(); err == nil {
				t.Fatal("invalid file accepted")
			}
			if _, err := s.SaveProject(Project{Name: "overwrite"}); err == nil {
				t.Fatal("invalid file overwritten")
			}
			back, _ := os.ReadFile(path)
			if string(back) != raw {
				t.Fatal("original file lost")
			}
		})
	}
}

func TestConcurrentChangesDoNotLoseTasks(t *testing.T) {
	s, d := setup(t)
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := NewStore(s.path).SaveTask(Task{ProjectID: d.Projects[0].ID, Title: "parallel", Status: "planned"})
			if err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	back, err := s.Snapshot()
	if err != nil || len(back.Tasks) != 13 {
		t.Fatalf("lost a task: %v %+v", err, back)
	}
}

func TestFailedWriteDoesNotPublishChange(t *testing.T) {
	s, d := setup(t)
	// A parent that is a regular file guarantees a write failure on every OS.
	bad := NewStore(filepath.Join(s.path, "child.json"))
	if _, err := bad.SaveProject(Project{Name: "cannot save"}); err == nil {
		t.Fatal("expected failure")
	}
	back, err := s.Snapshot()
	if err != nil || !reflect.DeepEqual(back, d) {
		t.Fatalf("original changed: %v", err)
	}
}
