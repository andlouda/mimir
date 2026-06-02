package aiflow

import (
	"os"
	"path/filepath"
	"testing"

	"mimir/tools"
	"mimir/workflow"
)

type stubTool struct {
	id       string
	name     string
	category string
	risk     tools.RiskLevel
	class    tools.ToolClass
}

func (s stubTool) ID() string                    { return s.id }
func (s stubTool) Name() string                  { return s.name }
func (s stubTool) Description() string           { return "" }
func (s stubTool) Category() string              { return s.category }
func (s stubTool) Risk() tools.RiskLevel         { return s.risk }
func (s stubTool) Class() tools.ToolClass        { return s.class }
func (s stubTool) Parameters() []tools.Parameter { return nil }
func (s stubTool) Run(tools.RunContext, map[string]string) (tools.ToolResult, error) {
	return tools.ToolResult{}, nil
}

func TestLoadConfigYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "ai_tool_flow.yaml")
	raw := []byte(`
prompt:
  requireStableToolId: true
  maxTerminalContext: 4000
toolFilter:
  includeCategories: ["Network", "Kubernetes"]
approval:
  requireApprovalForLow: true
execution:
  workflowName: "Configured AI Tool Run"
  workflowIdPrefix: "configured-ai"
`)
	if err := os.WriteFile(configPath, raw, 0600); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	originalEnv := os.Getenv(configEnvVar)
	t.Cleanup(func() {
		_ = os.Setenv(configEnvVar, originalEnv)
	})
	if err := os.Setenv(configEnvVar, configPath); err != nil {
		t.Fatalf("failed to set env: %v", err)
	}

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}

	if cfg.Prompt.MaxTerminalContext != 4000 {
		t.Fatalf("expected max context 4000, got %d", cfg.Prompt.MaxTerminalContext)
	}
	if cfg.Execution.WorkflowName != "Configured AI Tool Run" {
		t.Fatalf("unexpected workflow name: %s", cfg.Execution.WorkflowName)
	}
	if !cfg.Approval.RequireApprovalForLow {
		t.Fatalf("expected low risk approval to be enabled")
	}
}

func TestFilterTools(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ToolFilter.IncludeCategories = []string{"Network"}
	cfg.ToolFilter.ExcludeToolIDs = []string{"template:show_ip"}

	input := []tools.Tool{
		stubTool{id: "template:ping", name: "Ping", category: "Network", risk: tools.RiskLow},
		stubTool{id: "template:show_ip", name: "Show IP", category: "Network", risk: tools.RiskLow},
		stubTool{id: "template:disk", name: "Disk", category: "Storage", risk: tools.RiskLow},
	}

	filtered := cfg.FilterTools(input)
	if len(filtered) != 1 {
		t.Fatalf("expected 1 filtered tool, got %d", len(filtered))
	}
	if filtered[0].ID() != "template:ping" {
		t.Fatalf("unexpected filtered tool: %s", filtered[0].ID())
	}
}

func TestFilterToolsForProvider(t *testing.T) {
	cfg := DefaultConfig()
	input := []tools.Tool{
		stubTool{id: "template:ping", name: "Ping", category: "Network", risk: tools.RiskLow, class: tools.ClassSafeReadonly},
		stubTool{id: "template:k8s-logs", name: "K8s Logs", category: "Kubernetes", risk: tools.RiskLow, class: tools.ClassSensitiveReadonly},
	}

	openAI := cfg.FilterToolsForProvider(input, "openai")
	if len(openAI) != 1 || openAI[0].ID() != "template:ping" {
		t.Fatalf("expected OpenAI policy to allow only safe_readonly tools, got %+v", openAI)
	}

	ollama := cfg.FilterToolsForProvider(input, "ollama")
	if len(ollama) != 2 {
		t.Fatalf("expected Ollama policy to allow safe and sensitive readonly tools, got %d", len(ollama))
	}
}

func TestApprovalPolicyFromConfig(t *testing.T) {
	policy := NewApprovalPolicy(ApprovalConfig{
		RespectStepFlag:          true,
		RequireApprovalForLow:    false,
		RequireApprovalForMedium: true,
		RequireApprovalForHigh:   true,
	})

	decision := policy.Decide(workflow.ModeApprove, workflow.Step{}, stubTool{risk: tools.RiskMedium})
	if !decision.Required {
		t.Fatalf("expected medium risk approval to be required")
	}
	if decision.Risk != tools.RiskMedium {
		t.Fatalf("expected medium risk, got %s", decision.Risk)
	}
}

func TestSaveConfigAndLoadFromUserPath(t *testing.T) {
	tmpDir := t.TempDir()
	originalConfigHome := os.Getenv("XDG_CONFIG_HOME")
	originalEnv := os.Getenv(configEnvVar)
	t.Cleanup(func() {
		_ = os.Setenv("XDG_CONFIG_HOME", originalConfigHome)
		_ = os.Setenv(configEnvVar, originalEnv)
	})

	if err := os.Setenv("XDG_CONFIG_HOME", tmpDir); err != nil {
		t.Fatalf("failed to set config home: %v", err)
	}
	if err := os.Unsetenv(configEnvVar); err != nil {
		t.Fatalf("failed to clear config env: %v", err)
	}

	cfg := DefaultConfig()
	cfg.Prompt.PrePrompt = "Be conservative."
	cfg.Execution.WorkflowName = "Runtime AI Tool Run"

	if _, err := SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig returned error: %v", err)
	}

	loaded, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}

	if loaded.Prompt.PrePrompt != "Be conservative." {
		t.Fatalf("unexpected prePrompt: %q", loaded.Prompt.PrePrompt)
	}
	if loaded.Execution.WorkflowName != "Runtime AI Tool Run" {
		t.Fatalf("unexpected workflow name: %s", loaded.Execution.WorkflowName)
	}
}
