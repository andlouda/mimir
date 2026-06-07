package main

import (
	"strings"
	"testing"

	"mimir/aiflow"
	"mimir/tools"
)

type aiPromptTestTool struct{}

func (aiPromptTestTool) ID() string          { return "test.inspect" }
func (aiPromptTestTool) Name() string        { return "Inspect" }
func (aiPromptTestTool) Description() string { return "Inspect state" }
func (aiPromptTestTool) Category() string    { return "diagnostics" }
func (aiPromptTestTool) Risk() tools.RiskLevel {
	return tools.RiskLow
}
func (aiPromptTestTool) Class() tools.ToolClass { return tools.ClassSafeReadonly }
func (aiPromptTestTool) Parameters() []tools.Parameter {
	return []tools.Parameter{{Name: "Target", Required: true, Description: "Target to inspect"}}
}
func (aiPromptTestTool) Run(tools.RunContext, map[string]string) (tools.ToolResult, error) {
	return tools.ToolResult{}, nil
}

func TestBuildToolSelectionPromptOmitsTerminalOutputWhenDisabled(t *testing.T) {
	cfg := aiflow.DefaultConfig()
	cfg.Prompt.IncludeTerminalOutput = false

	prompt, err := buildToolSelectionPrompt(
		"inspect it",
		"bash",
		"Local",
		"SECRET_TERMINAL_OUTPUT",
		[]tools.Tool{aiPromptTestTool{}},
		cfg,
	)
	if err != nil {
		t.Fatalf("buildToolSelectionPrompt returned error: %v", err)
	}

	if strings.Contains(prompt, "SECRET_TERMINAL_OUTPUT") {
		t.Fatalf("prompt contains terminal output despite IncludeTerminalOutput=false:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Terminal type: bash") {
		t.Fatalf("prompt should still contain terminal metadata:\n%s", prompt)
	}
	if !strings.Contains(prompt, "test.inspect") {
		t.Fatalf("prompt should still contain tool metadata:\n%s", prompt)
	}
}

func TestBuildToolSelectionPromptIncludesTerminalOutputWhenEnabled(t *testing.T) {
	cfg := aiflow.DefaultConfig()
	cfg.Prompt.IncludeTerminalOutput = true

	prompt, err := buildToolSelectionPrompt(
		"inspect it",
		"bash",
		"Local",
		"VISIBLE_TERMINAL_OUTPUT",
		[]tools.Tool{aiPromptTestTool{}},
		cfg,
	)
	if err != nil {
		t.Fatalf("buildToolSelectionPrompt returned error: %v", err)
	}

	if !strings.Contains(prompt, "VISIBLE_TERMINAL_OUTPUT") {
		t.Fatalf("prompt should contain terminal output when IncludeTerminalOutput=true:\n%s", prompt)
	}
}
