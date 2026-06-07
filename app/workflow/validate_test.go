package workflow

import (
	"fmt"
	"strings"
	"testing"
)

func TestValidateDefinitionRejectsDuplicateStepIDs(t *testing.T) {
	err := ValidateDefinition(Definition{
		ID:   "workflow:test",
		Name: "Test",
		Mode: ModeAssist,
		Steps: []Step{
			{ID: "step-1", Type: StepRunTool, Tool: "template:one"},
			{ID: "step-1", Type: StepRunTool, Tool: "template:two"},
		},
	})
	if err == nil {
		t.Fatal("expected duplicate step IDs to be rejected")
	}
}

func TestValidateDefinitionRejectsInvalidStepConfig(t *testing.T) {
	err := ValidateDefinition(Definition{
		ID:   "workflow:test",
		Name: "Test",
		Mode: ModeAssist,
		Steps: []Step{
			{ID: "step-1", Type: StepRunDiscovery},
		},
	})
	if err == nil {
		t.Fatal("expected missing discovery tool to be rejected")
	}
}

func TestValidateStateForDefinitionRejectsMismatchedWorkflow(t *testing.T) {
	definition := Definition{
		ID:   "workflow:expected",
		Name: "Expected",
		Mode: ModeAssist,
		Steps: []Step{
			{ID: "step-1", Type: StepRunTool, Tool: "template:test"},
		},
	}
	state := &State{
		WorkflowID: "workflow:other",
	}

	if err := ValidateStateForDefinition(definition, state); err == nil {
		t.Fatal("expected mismatched workflow state to be rejected")
	}
}

func TestValidateStateForDefinitionRejectsUnknownPendingStep(t *testing.T) {
	definition := Definition{
		ID:   "workflow:expected",
		Name: "Expected",
		Mode: ModeAssist,
		Steps: []Step{
			{ID: "step-1", Type: StepRunTool, Tool: "template:test"},
		},
	}
	state := &State{
		WorkflowID: "workflow:expected",
		PendingApproval: &PendingApproval{
			StepID: "missing-step",
		},
	}

	if err := ValidateStateForDefinition(definition, state); err == nil {
		t.Fatal("expected unknown pending approval step to be rejected")
	}
}

func TestValidateDefinitionRejectsOversizedWorkflow(t *testing.T) {
	steps := make([]Step, 0, maxWorkflowSteps+1)
	for i := 0; i < maxWorkflowSteps+1; i++ {
		steps = append(steps, Step{
			ID:   fmt.Sprintf("step-overflow-%d", i),
			Type: StepRunTool,
			Tool: "template:test",
		})
	}

	err := ValidateDefinition(Definition{
		ID:    "workflow:test",
		Name:  "Test",
		Mode:  ModeAssist,
		Steps: steps,
	})
	if err == nil {
		t.Fatal("expected oversized workflow to be rejected")
	}
}

func TestValidateDefinitionRejectsOversizedInputValue(t *testing.T) {
	err := ValidateDefinition(Definition{
		ID:   "workflow:test",
		Name: "Test",
		Mode: ModeAssist,
		Steps: []Step{
			{
				ID:   "step-1",
				Type: StepRunTool,
				Tool: "template:test",
				Inputs: map[string]string{
					"path": strings.Repeat("x", maxInputValueLen+1),
				},
			},
		},
	})
	if err == nil {
		t.Fatal("expected oversized input value to be rejected")
	}
}
