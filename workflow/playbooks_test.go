package workflow

import "testing"

func TestDefaultPlaybooks(t *testing.T) {
	playbooks := DefaultPlaybooks()
	if len(playbooks) == 0 {
		t.Fatal("expected default playbooks")
	}

	seen := make(map[string]struct{}, len(playbooks))
	for _, playbook := range playbooks {
		if playbook.ID == "" {
			t.Fatal("expected playbook ID")
		}
		if _, ok := seen[playbook.ID]; ok {
			t.Fatalf("duplicate playbook id %s", playbook.ID)
		}
		seen[playbook.ID] = struct{}{}
		if len(playbook.Steps) == 0 {
			t.Fatalf("expected playbook %s to have steps", playbook.ID)
		}
		if playbook.Name == "" {
			t.Fatalf("expected playbook %s to have a name", playbook.ID)
		}
	}
}
