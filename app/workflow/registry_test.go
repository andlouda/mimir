package workflow

import "testing"

func TestRegistryRegisterGetList(t *testing.T) {
	registry := NewRegistry()

	second := Definition{ID: "workflow:b", Name: "Second", Mode: ModeAssist}
	first := Definition{ID: "workflow:a", Name: "First", Mode: ModeManual}

	if err := registry.Register(second); err != nil {
		t.Fatalf("register second workflow: %v", err)
	}
	if err := registry.Register(first); err != nil {
		t.Fatalf("register first workflow: %v", err)
	}

	got, ok := registry.Get("workflow:a")
	if !ok {
		t.Fatal("expected workflow to exist")
	}
	if got.Name != "First" {
		t.Fatalf("unexpected workflow name: %s", got.Name)
	}

	list := registry.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 workflows, got %d", len(list))
	}
	if list[0].ID != "workflow:a" || list[1].ID != "workflow:b" {
		t.Fatalf("expected deterministic sort order, got %+v", list)
	}
}

func TestRegistryRejectsDuplicateID(t *testing.T) {
	registry := NewRegistry()

	definition := Definition{ID: "workflow:dup", Name: "Duplicate", Mode: ModeManual}
	if err := registry.Register(definition); err != nil {
		t.Fatalf("register original workflow: %v", err)
	}
	if err := registry.Register(definition); err == nil {
		t.Fatal("expected duplicate workflow registration to fail")
	}
}
