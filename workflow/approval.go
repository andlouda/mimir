package workflow

import (
	"fmt"
	"time"

	"mimir/activitylog"
	"mimir/tools"
)

// ApprovalRequiredError signals that a workflow is paused pending user approval.
type ApprovalRequiredError struct {
	StepID string
	Reason string
}

func (e *ApprovalRequiredError) Error() string {
	return fmt.Sprintf("approval required for step %s: %s", e.StepID, e.Reason)
}

// ApprovalPolicy decides whether a step must pause for approval before execution.
type ApprovalPolicy interface {
	Decide(mode Mode, step Step, tool tools.Tool) ApprovalDecision
}

// DefaultApprovalPolicy is the conservative baseline policy for workflow execution.
type DefaultApprovalPolicy struct{}

// NewDefaultApprovalPolicy creates the standard workflow approval policy.
func NewDefaultApprovalPolicy() *DefaultApprovalPolicy {
	return &DefaultApprovalPolicy{}
}

// RejectPendingApproval marks a paused workflow step as denied and clears the approval state.
func RejectPendingApproval(state *State, reason string) error {
	if state == nil {
		return fmt.Errorf("workflow state is required")
	}
	if state.PendingApproval == nil {
		return fmt.Errorf("workflow is not waiting for approval")
	}

	denialReason := reason
	if denialReason == "" {
		denialReason = "approval denied by user"
	}

	state.Events = append(state.Events, Event{
		StepID:  state.PendingApproval.StepID,
		Type:    "approval_denied",
		Message: denialReason,
		Metadata: map[string]string{
			"tool_id":   state.PendingApproval.ToolID,
			"tool_name": state.PendingApproval.ToolName,
			"risk":      string(state.PendingApproval.Risk),
		},
	})
	_ = activitylog.Append(activitylog.KindApprovalEvents, activitylog.ApprovalEventEntry{
		Timestamp:  time.Now().Format(time.RFC3339),
		WorkflowID: state.WorkflowID,
		StepID:     state.PendingApproval.StepID,
		ToolID:     state.PendingApproval.ToolID,
		ToolName:   state.PendingApproval.ToolName,
		Risk:       string(state.PendingApproval.Risk),
		Event:      "denied",
		Reason:     denialReason,
	})
	state.PendingApproval = nil
	return nil
}

func (p *DefaultApprovalPolicy) Decide(mode Mode, step Step, tool tools.Tool) ApprovalDecision {
	risk := tools.RiskLow
	if tool != nil {
		risk = tool.Risk()
	}

	if step.RequiresApproval {
		return ApprovalDecision{
			Required: true,
			Risk:     risk,
			Reason:   "step explicitly requires approval",
		}
	}

	switch risk {
	case tools.RiskMedium:
		return ApprovalDecision{
			Required: true,
			Risk:     risk,
			Reason:   "medium-risk tools require approval",
		}
	case tools.RiskHigh:
		return ApprovalDecision{
			Required: true,
			Risk:     risk,
			Reason:   "high-risk tools require approval",
		}
	default:
		return ApprovalDecision{
			Required: false,
			Risk:     risk,
		}
	}
}
