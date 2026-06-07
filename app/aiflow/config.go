package aiflow

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"mimir/safeio"
	"mimir/tools"
	"mimir/workflow"

	"gopkg.in/yaml.v3"
)

const configEnvVar = "MIMIR_AI_TOOL_FLOW_CONFIG"
const userConfigFileName = "ai_tool_flow.json"

type PromptConfig struct {
	PrePrompt                 string `json:"prePrompt" yaml:"prePrompt"`
	RequireStableToolID       bool   `json:"requireStableToolId" yaml:"requireStableToolId"`
	IncludeRisk               bool   `json:"includeRisk" yaml:"includeRisk"`
	IncludeCategory           bool   `json:"includeCategory" yaml:"includeCategory"`
	IncludeTerminalOutput     bool   `json:"includeTerminalOutput" yaml:"includeTerminalOutput"`
	MaxTerminalContext        int    `json:"maxTerminalContext" yaml:"maxTerminalContext"`
	AllowTemplateNameFallback bool   `json:"allowTemplateNameFallback" yaml:"allowTemplateNameFallback"`
}

type ToolFilterConfig struct {
	IncludeCategories []string `json:"includeCategories" yaml:"includeCategories"`
	ExcludeCategories []string `json:"excludeCategories" yaml:"excludeCategories"`
	IncludeToolIDs    []string `json:"includeToolIds" yaml:"includeToolIds"`
	ExcludeToolIDs    []string `json:"excludeToolIds" yaml:"excludeToolIds"`
}

type ApprovalConfig struct {
	RespectStepFlag          bool `json:"respectStepFlag" yaml:"respectStepFlag"`
	RequireApprovalForLow    bool `json:"requireApprovalForLow" yaml:"requireApprovalForLow"`
	RequireApprovalForMedium bool `json:"requireApprovalForMedium" yaml:"requireApprovalForMedium"`
	RequireApprovalForHigh   bool `json:"requireApprovalForHigh" yaml:"requireApprovalForHigh"`
}

type ExecutionConfig struct {
	Enabled               bool          `json:"enabled" yaml:"enabled"`
	WorkflowMode          workflow.Mode `json:"workflowMode" yaml:"workflowMode"`
	WorkflowIDPrefix      string        `json:"workflowIdPrefix" yaml:"workflowIdPrefix"`
	WorkflowName          string        `json:"workflowName" yaml:"workflowName"`
	ForceRequiresApproval bool          `json:"forceRequiresApproval" yaml:"forceRequiresApproval"`
}

type ProviderPolicyConfig struct {
	AllowedToolClasses    []tools.ToolClass `json:"allowedToolClasses" yaml:"allowedToolClasses"`
	AllowSensitiveContext bool              `json:"allowSensitiveContext" yaml:"allowSensitiveContext"`
	MaxContextChars       int               `json:"maxContextChars" yaml:"maxContextChars"`
}

type Config struct {
	Prompt           PromptConfig                    `json:"prompt" yaml:"prompt"`
	ToolFilter       ToolFilterConfig                `json:"toolFilter" yaml:"toolFilter"`
	Approval         ApprovalConfig                  `json:"approval" yaml:"approval"`
	Execution        ExecutionConfig                 `json:"execution" yaml:"execution"`
	ProviderPolicies map[string]ProviderPolicyConfig `json:"providerPolicies,omitempty" yaml:"providerPolicies,omitempty"`
}

func DefaultConfig() Config {
	return Config{
		Prompt: PromptConfig{
			PrePrompt:                 "",
			RequireStableToolID:       true,
			IncludeRisk:               true,
			IncludeCategory:           true,
			IncludeTerminalOutput:     true,
			MaxTerminalContext:        12000,
			AllowTemplateNameFallback: false,
		},
		ToolFilter: ToolFilterConfig{},
		Approval: ApprovalConfig{
			RespectStepFlag:          true,
			RequireApprovalForLow:    false,
			RequireApprovalForMedium: true,
			RequireApprovalForHigh:   true,
		},
		Execution: ExecutionConfig{
			Enabled:               true,
			WorkflowMode:          workflow.ModeApprove,
			WorkflowIDPrefix:      "ai-tool-run",
			WorkflowName:          "AI Tool Run",
			ForceRequiresApproval: false,
		},
		ProviderPolicies: defaultProviderPolicies(),
	}
}

func LoadConfig() (Config, error) {
	defaults := DefaultConfig()

	path, ok := resolveConfigPath()
	if !ok {
		return defaults, nil
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("failed to read AI tool flow config: %w", err)
	}

	cfg := defaults
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(raw, &cfg); err != nil {
			return Config{}, fmt.Errorf("failed to parse AI tool flow yaml: %w", err)
		}
	default:
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return Config{}, fmt.Errorf("failed to parse AI tool flow json: %w", err)
		}
	}

	cfg.normalize()
	return cfg, nil
}

func SaveConfig(cfg Config) (Config, error) {
	path, err := writableConfigPath()
	if err != nil {
		return Config{}, err
	}

	cfg.normalize()

	var payload []byte
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		payload, err = yaml.Marshal(cfg)
		if err != nil {
			return Config{}, fmt.Errorf("failed to encode AI tool flow yaml: %w", err)
		}
	default:
		payload, err = json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return Config{}, fmt.Errorf("failed to encode AI tool flow json: %w", err)
		}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return Config{}, fmt.Errorf("failed to create AI tool flow config directory: %w", err)
	}
	if err := safeio.AtomicWriteFile(path, payload, 0600); err != nil {
		return Config{}, fmt.Errorf("failed to write AI tool flow config: %w", err)
	}

	return cfg, nil
}

func resolveConfigPath() (string, bool) {
	if configured := strings.TrimSpace(os.Getenv(configEnvVar)); configured != "" {
		return configured, true
	}

	userPath, err := userConfigPath()
	if err == nil {
		if _, err := os.Stat(userPath); err == nil {
			return userPath, true
		}
	}

	candidates := []string{
		filepath.Join("config", "ai_tool_flow.yaml"),
		filepath.Join("config", "ai_tool_flow.yml"),
		filepath.Join("config", "ai_tool_flow.json"),
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, true
		}
	}

	return "", false
}

func writableConfigPath() (string, error) {
	if configured := strings.TrimSpace(os.Getenv(configEnvVar)); configured != "" {
		return configured, nil
	}
	return userConfigPath()
}

func userConfigPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user config directory: %w", err)
	}
	return filepath.Join(configDir, "mimir", userConfigFileName), nil
}

func (c *Config) normalize() {
	defaults := DefaultConfig()

	if c.Prompt.MaxTerminalContext <= 0 {
		c.Prompt.MaxTerminalContext = 12000
	}
	if strings.TrimSpace(string(c.Execution.WorkflowMode)) == "" {
		c.Execution.WorkflowMode = workflow.ModeApprove
	}
	if strings.TrimSpace(c.Execution.WorkflowIDPrefix) == "" {
		c.Execution.WorkflowIDPrefix = "ai-tool-run"
	}
	if strings.TrimSpace(c.Execution.WorkflowName) == "" {
		c.Execution.WorkflowName = "AI Tool Run"
	}
	if len(c.ProviderPolicies) == 0 {
		c.ProviderPolicies = defaults.ProviderPolicies
		return
	}

	for provider, policy := range defaults.ProviderPolicies {
		existing, ok := c.ProviderPolicies[provider]
		if !ok {
			c.ProviderPolicies[provider] = policy
			continue
		}
		if len(existing.AllowedToolClasses) == 0 {
			existing.AllowedToolClasses = policy.AllowedToolClasses
		}
		if existing.MaxContextChars <= 0 {
			existing.MaxContextChars = policy.MaxContextChars
		}
		c.ProviderPolicies[provider] = existing
	}
}

func (c Config) FilterTools(available []tools.Tool) []tools.Tool {
	result := make([]tools.Tool, 0, len(available))
	for _, tool := range available {
		if tool == nil {
			continue
		}
		if len(c.ToolFilter.IncludeCategories) > 0 && !containsFold(c.ToolFilter.IncludeCategories, tool.Category()) {
			continue
		}
		if containsFold(c.ToolFilter.ExcludeCategories, tool.Category()) {
			continue
		}
		if len(c.ToolFilter.IncludeToolIDs) > 0 && !containsFold(c.ToolFilter.IncludeToolIDs, tool.ID()) {
			continue
		}
		if containsFold(c.ToolFilter.ExcludeToolIDs, tool.ID()) {
			continue
		}
		result = append(result, tool)
	}
	return result
}

func containsFold(values []string, target string) bool {
	return slices.ContainsFunc(values, func(value string) bool {
		return strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(target))
	})
}

func defaultProviderPolicies() map[string]ProviderPolicyConfig {
	return map[string]ProviderPolicyConfig{
		"openai": {
			AllowedToolClasses:    []tools.ToolClass{tools.ClassSafeReadonly},
			AllowSensitiveContext: false,
			MaxContextChars:       4000,
		},
		"ollama": {
			AllowedToolClasses:    []tools.ToolClass{tools.ClassSafeReadonly, tools.ClassSensitiveReadonly},
			AllowSensitiveContext: true,
			MaxContextChars:       12000,
		},
		// Cloud provider — conservative, like OpenAI.
		"anthropic": {
			AllowedToolClasses:    []tools.ToolClass{tools.ClassSafeReadonly},
			AllowSensitiveContext: false,
			MaxContextChars:       4000,
		},
		// Custom/unknown OpenAI-compatible endpoint — treat as untrusted: no
		// sensitive context, safe read-only tools only. Users can loosen this
		// per their own config if the endpoint is a trusted local server.
		"openai_compatible": {
			AllowedToolClasses:    []tools.ToolClass{tools.ClassSafeReadonly},
			AllowSensitiveContext: false,
			MaxContextChars:       4000,
		},
	}
}

func (c Config) ProviderPolicy(provider string) ProviderPolicyConfig {
	defaults := defaultProviderPolicies()
	if policy, ok := c.ProviderPolicies[strings.ToLower(strings.TrimSpace(provider))]; ok {
		if len(policy.AllowedToolClasses) == 0 {
			policy.AllowedToolClasses = defaults[strings.ToLower(strings.TrimSpace(provider))].AllowedToolClasses
		}
		if policy.MaxContextChars <= 0 {
			policy.MaxContextChars = defaults[strings.ToLower(strings.TrimSpace(provider))].MaxContextChars
		}
		return policy
	}
	if policy, ok := defaults[strings.ToLower(strings.TrimSpace(provider))]; ok {
		return policy
	}
	return ProviderPolicyConfig{
		AllowedToolClasses:    []tools.ToolClass{tools.ClassSafeReadonly},
		AllowSensitiveContext: false,
		MaxContextChars:       4000,
	}
}

func (c Config) FilterToolsForProvider(available []tools.Tool, provider string) []tools.Tool {
	policy := c.ProviderPolicy(provider)
	filtered := c.FilterTools(available)
	if len(policy.AllowedToolClasses) == 0 {
		return filtered
	}

	result := make([]tools.Tool, 0, len(filtered))
	for _, tool := range filtered {
		if tool == nil {
			continue
		}
		for _, class := range policy.AllowedToolClasses {
			if tool.Class() == class {
				result = append(result, tool)
				break
			}
		}
	}
	return result
}

type ApprovalPolicy struct {
	config ApprovalConfig
}

func NewApprovalPolicy(config ApprovalConfig) *ApprovalPolicy {
	return &ApprovalPolicy{config: config}
}

func (p *ApprovalPolicy) Decide(_ workflow.Mode, step workflow.Step, tool tools.Tool) workflow.ApprovalDecision {
	risk := tools.RiskLow
	if tool != nil {
		risk = tool.Risk()
	}

	if p.config.RespectStepFlag && step.RequiresApproval {
		return workflow.ApprovalDecision{
			Required: true,
			Risk:     risk,
			Reason:   "step explicitly requires approval",
		}
	}

	required := false
	reason := ""
	switch risk {
	case tools.RiskLow:
		required = p.config.RequireApprovalForLow
		if required {
			reason = "low-risk tools require approval by AI flow policy"
		}
	case tools.RiskMedium:
		required = p.config.RequireApprovalForMedium
		if required {
			reason = "medium-risk tools require approval by AI flow policy"
		}
	case tools.RiskHigh:
		required = p.config.RequireApprovalForHigh
		if required {
			reason = "high-risk tools require approval by AI flow policy"
		}
	}

	return workflow.ApprovalDecision{
		Required: required,
		Risk:     risk,
		Reason:   reason,
	}
}
