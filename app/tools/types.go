package tools

import "mimir/template"

// RiskLevel describes how sensitive a tool execution is.
type RiskLevel string

const (
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

// ToolClass describes the highest-level execution sensitivity of a tool.
type ToolClass string

const (
	ClassSafeReadonly      ToolClass = "safe_readonly"
	ClassSensitiveReadonly ToolClass = "sensitive_readonly"
	ClassMutating          ToolClass = "mutating"
	ClassDestructive       ToolClass = "destructive"
	ClassSecretAccess      ToolClass = "secret_access"
)

// ParameterSource defines where a parameter value may come from.
type ParameterSource string

const (
	ParameterSourceAIAllowed     ParameterSource = "ai_allowed"
	ParameterSourceUserOnly      ParameterSource = "user_only"
	ParameterSourceDiscoveryOnly ParameterSource = "discovery_only"
)

// Parameter describes a named tool input.
type Parameter struct {
	Name          string          `json:"name"`
	Description   string          `json:"description,omitempty"`
	Required      bool            `json:"required"`
	Type          string          `json:"type,omitempty"`
	Pattern       string          `json:"pattern,omitempty"`
	MaxLength     int             `json:"maxLength,omitempty"`
	Options       []string        `json:"options,omitempty"`
	Source        ParameterSource `json:"source,omitempty"`
	DiscoveryTool string          `json:"discoveryTool,omitempty"`
}

// ToolResult is the normalized output of a tool run.
type ToolResult struct {
	Output   string            `json:"output,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// RunContext carries execution dependencies for tools.
type RunContext struct {
	TerminalID   int
	TerminalType string
	Writer       template.Writer
	TemplateData template.TemplateContext
}

// Tool is the minimal execution contract used by workflows.
type Tool interface {
	ID() string
	Name() string
	Description() string
	Category() string
	Risk() RiskLevel
	Class() ToolClass
	Parameters() []Parameter
	Run(ctx RunContext, input map[string]string) (ToolResult, error)
}

// GuardrailInspectable exposes deterministic metadata for backend validation.
type GuardrailInspectable interface {
	Tool
	CommandMap() map[string]string
}
