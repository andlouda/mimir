package workflow

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPlaybookStoreListSeedsDefaults(t *testing.T) {
	store := NewPlaybookStore(t.TempDir(), DefaultPlaybooks())

	playbooks, err := store.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(playbooks) != len(DefaultPlaybooks()) {
		t.Fatalf("expected %d playbooks, got %d", len(DefaultPlaybooks()), len(playbooks))
	}

	for _, playbook := range DefaultPlaybooks() {
		if _, err := os.Stat(filepath.Join(store.dir, sanitizePlaybookID(playbook.ID)+playbookFileExt)); err != nil {
			t.Fatalf("expected seeded file for %s: %v", playbook.ID, err)
		}
	}
}

func TestPlaybookStoreSaveRenameAndDelete(t *testing.T) {
	store := NewPlaybookStore(t.TempDir(), nil)

	saved, err := store.Save(Definition{
		Name:        "My Host Triage",
		Description: "Custom host checks",
		Mode:        ModeAssist,
		Steps: []Step{
			{Type: StepRunTool, Tool: "template:System Resources"},
		},
	}, "")
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if saved.ID != "playbook:my-host-triage" {
		t.Fatalf("unexpected saved id %s", saved.ID)
	}

	renamed, err := store.Save(Definition{
		Name:        "Renamed Host Triage",
		Description: "Custom host checks",
		Mode:        ModeAssist,
		Steps: []Step{
			{Type: StepRunTool, Tool: "template:System Resources"},
		},
	}, saved.ID)
	if err != nil {
		t.Fatalf("Save(rename) error = %v", err)
	}
	if renamed.ID != "playbook:renamed-host-triage" {
		t.Fatalf("unexpected renamed id %s", renamed.ID)
	}
	if _, err := os.Stat(filepath.Join(store.dir, sanitizePlaybookID(saved.ID)+playbookFileExt)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected previous playbook file to be removed, stat err = %v", err)
	}

	playbooks, err := store.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(playbooks) != 1 {
		t.Fatalf("expected one playbook after rename, got %d", len(playbooks))
	}

	if err := store.Delete(renamed.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	playbooks, err = store.List()
	if err != nil {
		t.Fatalf("List() after delete error = %v", err)
	}
	if len(playbooks) != 0 {
		t.Fatalf("expected zero playbooks after delete, got %d", len(playbooks))
	}
}

func TestPlaybookStoreSaveRequiresNameAndStep(t *testing.T) {
	store := NewPlaybookStore(t.TempDir(), nil)

	if _, err := store.Save(Definition{}, ""); err == nil {
		t.Fatal("expected error for empty playbook")
	}

	if _, err := store.Save(Definition{Name: "Empty", Mode: ModeAssist}, ""); err == nil {
		t.Fatal("expected error for playbook without steps")
	}
}

func TestPlaybookStoreSaveNormalizesPlaybookPrefix(t *testing.T) {
	store := NewPlaybookStore(t.TempDir(), nil)

	saved, err := store.Save(Definition{
		ID:   "custom id",
		Name: "Custom ID",
		Mode: ModeAssist,
		Steps: []Step{
			{Type: StepRunTool, Tool: "template:System Resources"},
		},
	}, "")
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if saved.ID != "playbook:custom-id" {
		t.Fatalf("expected normalized playbook prefix, got %s", saved.ID)
	}
}

func TestPlaybookStoreProtectsDefaultPlaybooks(t *testing.T) {
	store := NewPlaybookStore(t.TempDir(), DefaultPlaybooks())

	defaults, err := store.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	protected := defaults[0]

	if !store.IsProtectedID(protected.ID) {
		t.Fatalf("expected %s to be protected", protected.ID)
	}

	if _, err := store.Save(Definition{
		ID:          protected.ID,
		Name:        protected.Name,
		Description: "mutated",
		Mode:        protected.Mode,
		Steps:       protected.Steps,
	}, protected.ID); err == nil {
		t.Fatal("expected in-place save of protected playbook to fail")
	}

	if err := store.Delete(protected.ID); err == nil {
		t.Fatal("expected delete of protected playbook to fail")
	}

	duplicate, err := store.Save(Definition{
		Name:        protected.Name + " Copy",
		Description: protected.Description,
		Mode:        protected.Mode,
		Steps:       protected.Steps,
	}, protected.ID)
	if err != nil {
		t.Fatalf("expected duplicate of protected playbook to succeed, got %v", err)
	}
	if duplicate.ID == protected.ID {
		t.Fatal("expected duplicate playbook to get a new ID")
	}
}
