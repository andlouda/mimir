package workflow

import "mimir/tools"

// Mode defines how much autonomy a workflow gets.
type Mode string

const (
	ModeManual  Mode = "manual"
	ModeAssist  Mode = "assist"
	ModeApprove Mode = "approve"
	ModeAuto    Mode = "auto"
)

// StepType describes the executor category for a workflow step.
type StepType string

const (
	StepRunTool      StepType = "run_tool"
	StepRunDiscovery StepType = "run_discovery"
	StepAskAI        StepType = "ask_ai"
	StepAskUser      StepType = "ask_user"
)

// Step is a serializable workflow instruction.
type Step struct {
	ID               string            `json:"id" yaml:"id"`
	Type             StepType          `json:"type" yaml:"type"`
	Tool             string            `json:"tool,omitempty" yaml:"tool,omitempty"`
	DiscoveryTool    string            `json:"discoveryTool,omitempty" yaml:"discoveryTool,omitempty"`
	Prompt           string            `json:"prompt,omitempty" yaml:"prompt,omitempty"`
	Inputs           map[string]string `json:"inputs,omitempty" yaml:"inputs,omitempty"`
	RequiresApproval bool              `json:"requiresApproval,omitempty" yaml:"requiresApproval,omitempty"`
	AIMode           string            `json:"aiMode,omitempty" yaml:"aiMode,omitempty"`
}

// Definition is the top-level workflow configuration.
type Definition struct {
	ID          string `json:"id" yaml:"id"`
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description" yaml:"description"`
	Mode        Mode   `json:"mode" yaml:"mode"`
	Steps       []Step `json:"steps" yaml:"steps"`
}

// Event captures workflow progress in a serializable form.
type Event struct {
	StepID   string            `json:"stepId"`
	Type     string            `json:"type"`
	Message  string            `json:"message"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// PendingApproval captures a paused step waiting for user confirmation.
type PendingApproval struct {
	StepID   string          `json:"stepId"`
	ToolID   string          `json:"toolId"`
	ToolName string          `json:"toolName"`
	Risk     tools.RiskLevel `json:"risk"`
	Reason   string          `json:"reason"`
}

// State is the mutable runtime shape for a workflow run.
type State struct {
	WorkflowID      string              `json:"workflowId"`
	StepIndex       int                 `json:"stepIndex"`
	Inputs          map[string]string   `json:"inputs,omitempty"`
	Outputs         map[string]string   `json:"outputs,omitempty"`
	Discovery       map[string][]string `json:"discovery,omitempty"`
	Events          []Event             `json:"events,omitempty"`
	PendingApproval *PendingApproval    `json:"pendingApproval,omitempty"`
	ApprovedSteps   map[string]bool     `json:"approvedSteps,omitempty"`
}

// DiscoveryResolver resolves read-only discovery values for workflow steps.
type DiscoveryResolver interface {
	Resolve(discoveryTool string, terminalType string, variables map[string]string) ([]string, error)
}

// AIRunner executes an AI reasoning step with terminal context.
type AIRunner interface {
	Run(prompt string, terminalType string, terminalName string, terminalOutput string) (string, error)
}

// ApprovalDecision is the normalized result from policy checks.
type ApprovalDecision struct {
	Required bool            `json:"required"`
	Risk     tools.RiskLevel `json:"risk"`
	Reason   string          `json:"reason,omitempty"`
}
