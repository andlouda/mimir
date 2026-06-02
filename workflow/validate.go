package workflow

import (
	"fmt"
	"strings"
)

const (
	maxWorkflowSteps       = 64
	maxStepInputs          = 24
	maxWorkflowNameLen     = 160
	maxWorkflowDescLen     = 2000
	maxWorkflowIDLen       = 120
	maxStepIDLen           = 120
	maxPromptLen           = 4000
	maxInputKeyLen         = 80
	maxInputValueLen       = 1000
	maxToolRefLen          = 180
	maxDiscoveryToolRefLen = 180
)

func ValidateDefinition(def Definition) error {
	if strings.TrimSpace(def.ID) == "" {
		return fmt.Errorf("workflow definition requires an ID")
	}
	if strings.TrimSpace(def.Name) == "" {
		return fmt.Errorf("workflow definition requires a name")
	}
	if len(def.ID) > maxWorkflowIDLen {
		return fmt.Errorf("workflow %s exceeds maximum id length", def.ID)
	}
	if len(def.Name) > maxWorkflowNameLen {
		return fmt.Errorf("workflow %s exceeds maximum name length", def.ID)
	}
	if len(def.Description) > maxWorkflowDescLen {
		return fmt.Errorf("workflow %s exceeds maximum description length", def.ID)
	}
	switch def.Mode {
	case ModeManual, ModeAssist, ModeApprove, ModeAuto:
	default:
		return fmt.Errorf("workflow %s has unsupported mode %q", def.ID, def.Mode)
	}
	if len(def.Steps) == 0 {
		return fmt.Errorf("workflow %s requires at least one step", def.ID)
	}
	if len(def.Steps) > maxWorkflowSteps {
		return fmt.Errorf("workflow %s exceeds maximum step count", def.ID)
	}

	seen := make(map[string]struct{}, len(def.Steps))
	for index, step := range def.Steps {
		stepID := strings.TrimSpace(step.ID)
		if stepID == "" {
			return fmt.Errorf("workflow %s step %d requires an ID", def.ID, index+1)
		}
		if len(stepID) > maxStepIDLen {
			return fmt.Errorf("workflow %s step %s exceeds maximum id length", def.ID, stepID)
		}
		if _, ok := seen[stepID]; ok {
			return fmt.Errorf("workflow %s has duplicate step ID %s", def.ID, stepID)
		}
		seen[stepID] = struct{}{}

		switch step.Type {
		case StepRunTool:
			if strings.TrimSpace(step.Tool) == "" {
				return fmt.Errorf("workflow %s step %s requires a tool", def.ID, stepID)
			}
			if len(step.Tool) > maxToolRefLen {
				return fmt.Errorf("workflow %s step %s tool reference is too long", def.ID, stepID)
			}
		case StepRunDiscovery:
			if strings.TrimSpace(step.DiscoveryTool) == "" {
				return fmt.Errorf("workflow %s step %s requires a discovery tool", def.ID, stepID)
			}
			if len(step.DiscoveryTool) > maxDiscoveryToolRefLen {
				return fmt.Errorf("workflow %s step %s discovery tool reference is too long", def.ID, stepID)
			}
		case StepAskAI:
			if strings.TrimSpace(step.Prompt) == "" {
				return fmt.Errorf("workflow %s step %s requires an AI prompt", def.ID, stepID)
			}
			if len(step.Prompt) > maxPromptLen {
				return fmt.Errorf("workflow %s step %s prompt exceeds maximum length", def.ID, stepID)
			}
		case StepAskUser:
			if strings.TrimSpace(step.Prompt) == "" {
				return fmt.Errorf("workflow %s step %s requires a user prompt", def.ID, stepID)
			}
			if len(step.Prompt) > maxPromptLen {
				return fmt.Errorf("workflow %s step %s prompt exceeds maximum length", def.ID, stepID)
			}
		default:
			return fmt.Errorf("workflow %s step %s uses unsupported type %q", def.ID, stepID, step.Type)
		}

		if len(step.Inputs) > maxStepInputs {
			return fmt.Errorf("workflow %s step %s exceeds maximum input count", def.ID, stepID)
		}
		for key := range step.Inputs {
			if strings.TrimSpace(key) == "" {
				return fmt.Errorf("workflow %s step %s contains an empty input key", def.ID, stepID)
			}
			if len(key) > maxInputKeyLen {
				return fmt.Errorf("workflow %s step %s contains an oversized input key", def.ID, stepID)
			}
			if len(step.Inputs[key]) > maxInputValueLen {
				return fmt.Errorf("workflow %s step %s input %s exceeds maximum value length", def.ID, stepID, key)
			}
		}
	}

	return nil
}

func ValidateStateForDefinition(def Definition, state *State) error {
	if state == nil {
		return fmt.Errorf("workflow state is required")
	}
	if strings.TrimSpace(state.WorkflowID) != strings.TrimSpace(def.ID) {
		return fmt.Errorf("workflow state does not match definition: state=%s definition=%s", state.WorkflowID, def.ID)
	}
	if state.StepIndex < 0 || state.StepIndex > len(def.Steps) {
		return fmt.Errorf("workflow state has invalid step index %d", state.StepIndex)
	}
	if state.PendingApproval == nil {
		return nil
	}

	for _, step := range def.Steps {
		if step.ID == state.PendingApproval.StepID {
			return nil
		}
	}
	return fmt.Errorf("workflow state references unknown pending approval step %s", state.PendingApproval.StepID)
}
