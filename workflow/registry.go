package workflow

import (
	"fmt"
	"sort"
)

// Registry stores workflow definitions by stable ID.
type Registry struct {
	definitions map[string]Definition
}

// NewRegistry returns an empty workflow registry.
func NewRegistry() *Registry {
	return &Registry{
		definitions: make(map[string]Definition),
	}
}

// Register adds a workflow definition to the registry.
func (r *Registry) Register(def Definition) error {
	if def.ID == "" {
		return fmt.Errorf("workflow ID cannot be empty")
	}
	if _, exists := r.definitions[def.ID]; exists {
		return fmt.Errorf("workflow with ID %s already registered", def.ID)
	}
	r.definitions[def.ID] = def
	return nil
}

// Get looks up a workflow by ID.
func (r *Registry) Get(id string) (Definition, bool) {
	def, ok := r.definitions[id]
	return def, ok
}

// List returns workflow definitions sorted by ID.
func (r *Registry) List() []Definition {
	ids := make([]string, 0, len(r.definitions))
	for id := range r.definitions {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	result := make([]Definition, 0, len(ids))
	for _, id := range ids {
		result = append(result, r.definitions[id])
	}
	return result
}
