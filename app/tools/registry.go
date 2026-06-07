package tools

import (
	"fmt"
	"sort"
)

// Registry stores tools by stable ID.
type Registry struct {
	tools map[string]Tool
}

// NewRegistry creates an empty tool registry.
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// Register adds a tool to the registry.
func (r *Registry) Register(tool Tool) error {
	if tool == nil {
		return fmt.Errorf("tool cannot be nil")
	}
	if tool.ID() == "" {
		return fmt.Errorf("tool ID cannot be empty")
	}
	if _, exists := r.tools[tool.ID()]; exists {
		return fmt.Errorf("tool with ID %s already registered", tool.ID())
	}

	r.tools[tool.ID()] = tool
	return nil
}

// Get returns a tool by ID.
func (r *Registry) Get(id string) (Tool, bool) {
	tool, ok := r.tools[id]
	return tool, ok
}

// List returns tools sorted by ID for deterministic iteration.
func (r *Registry) List() []Tool {
	ids := make([]string, 0, len(r.tools))
	for id := range r.tools {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	result := make([]Tool, 0, len(ids))
	for _, id := range ids {
		result = append(result, r.tools[id])
	}
	return result
}
