package tools

import "testing"

type stubTool struct {
	id   string
	name string
}

func (s stubTool) ID() string              { return s.id }
func (s stubTool) Name() string            { return s.name }
func (s stubTool) Description() string     { return "" }
func (s stubTool) Category() string        { return "" }
func (s stubTool) Risk() RiskLevel         { return RiskLow }
func (s stubTool) Class() ToolClass        { return ClassSafeReadonly }
func (s stubTool) Parameters() []Parameter { return nil }
func (s stubTool) Run(RunContext, map[string]string) (ToolResult, error) {
	return ToolResult{}, nil
}

func TestRegistryRegisterGetList(t *testing.T) {
	registry := NewRegistry()

	first := stubTool{id: "b", name: "Second"}
	second := stubTool{id: "a", name: "First"}

	if err := registry.Register(first); err != nil {
		t.Fatalf("register first tool: %v", err)
	}
	if err := registry.Register(second); err != nil {
		t.Fatalf("register second tool: %v", err)
	}

	got, ok := registry.Get("a")
	if !ok {
		t.Fatal("expected tool with ID a to be present")
	}
	if got.Name() != "First" {
		t.Fatalf("unexpected tool name: %s", got.Name())
	}

	list := registry.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(list))
	}
	if list[0].ID() != "a" || list[1].ID() != "b" {
		t.Fatalf("expected deterministic sort order, got %s then %s", list[0].ID(), list[1].ID())
	}
}

func TestRegistryRejectsDuplicateIDs(t *testing.T) {
	registry := NewRegistry()

	if err := registry.Register(stubTool{id: "dup"}); err != nil {
		t.Fatalf("register original tool: %v", err)
	}
	if err := registry.Register(stubTool{id: "dup"}); err == nil {
		t.Fatal("expected duplicate tool registration to fail")
	}
}
