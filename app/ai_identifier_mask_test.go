package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// freshConfigDir gives the test its own data directory inside the sandbox,
// so the interaction log holds only this test's entries.
func freshConfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"HOME", "USERPROFILE", "XDG_CONFIG_HOME", "APPDATA"} {
		t.Setenv(name, dir)
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("UserConfigDir: %v", err)
	}
	return filepath.Join(configDir, "mimir")
}

func maskTestApp(provider string, caller func(input string) (string, error)) *App {
	return &App{
		aiSettings: AISettings{Provider: provider, Model: "m", BaseURL: "unused", APIKey: "k"},
		aiProviderCaller: func(_ AISettings, input string) (string, error) {
			return caller(input)
		},
	}
}

func askText(t *testing.T, app *App, mode, goal, output string) aiAskResult {
	t.Helper()
	payload, err := app.AskAI(mode, goal, "bash", "Bash", output)
	if err != nil {
		t.Fatalf("AskAI: %v", err)
	}
	var parsed aiAskResult
	if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
		t.Fatalf("decode AskAI payload: %v", err)
	}
	return parsed
}

func TestAskAIMasksIdentifiersForCloudProvidersAndTranslatesBack(t *testing.T) {
	configDir := freshConfigDir(t)
	var sent string
	app := maskTestApp(aiProviderOpenAI, func(input string) (string, error) {
		sent = input
		return "ssh admin@MIMIR_IP_1 'systemctl status nginx'", nil
	})

	result := askText(t, app, "write_command_from_goal", "check nginx on 10.0.3.17, then mail ops@corp.example",
		"curl: (7) Failed to connect to 10.0.3.17 port 80\nroute via 10.0.3.1")

	for _, leak := range []string{"10.0.3.17", "10.0.3.1", "ops@corp.example"} {
		if strings.Contains(sent, leak) {
			t.Errorf("%q left the machine: %s", leak, sent)
		}
	}
	if !strings.Contains(sent, "MIMIR_IP_1 port 80") || !strings.Contains(sent, "check nginx on MIMIR_IP_1") {
		t.Errorf("prompt does not carry consistent placeholders: %s", sent)
	}
	if !strings.Contains(sent, "stand for real addresses") {
		t.Errorf("prompt does not explain the placeholders: %s", sent)
	}
	if result.Text != "ssh admin@10.0.3.17 'systemctl status nginx'" || result.Warning != "" {
		t.Fatalf("result = %#v, want the command with the real address", result)
	}

	// The audit log records what left the machine and counts, not values.
	logData, err := os.ReadFile(filepath.Join(configDir, "ai_interactions.jsonl"))
	if err != nil {
		t.Fatalf("read interaction log: %v", err)
	}
	if strings.Contains(string(logData), "10.0.3.17") || strings.Contains(string(logData), "ops@corp.example") {
		t.Errorf("interaction log contains the real values: %s", logData)
	}
	if !strings.Contains(string(logData), `"maskedIdentifiers"`) {
		t.Errorf("interaction log has no masking counts: %s", logData)
	}
}

func TestAskAIWarnsAboutPlaceholdersTheModelInvented(t *testing.T) {
	freshConfigDir(t)
	app := maskTestApp(aiProviderOpenAI, func(string) (string, error) {
		return "ping -c1 MIMIR_IP_9", nil
	})
	result := askText(t, app, "suggest_next_command", "", "host 10.0.3.17 unreachable")
	if result.Text != "ping -c1 MIMIR_IP_9" {
		t.Fatalf("text = %q, want the unresolved placeholder left visible", result.Text)
	}
	if !strings.Contains(result.Warning, "MIMIR_IP_9") {
		t.Fatalf("warning = %q, want the unresolved placeholder named", result.Warning)
	}
}

func TestAskAIDoesNotMaskForLocalProviders(t *testing.T) {
	freshConfigDir(t)
	var sent string
	app := maskTestApp(aiProviderOllama, func(input string) (string, error) {
		sent = input
		return "ping -c1 10.0.3.17", nil
	})
	result := askText(t, app, "suggest_next_command", "", "host 10.0.3.17 unreachable")
	if !strings.Contains(sent, "10.0.3.17") || strings.Contains(sent, "MIMIR_IP_") {
		t.Errorf("local provider should see the real output: %s", sent)
	}
	if result.Text != "ping -c1 10.0.3.17" || result.Warning != "" {
		t.Fatalf("result = %#v", result)
	}
}

// TestWorkflowAIStepNeverSendsRawTerminalOutput is the regression test for
// the built-in "AI mode" prompt, which embedded the unsanitized output.
func TestWorkflowAIStepNeverSendsRawTerminalOutput(t *testing.T) {
	freshConfigDir(t)
	var sent string
	app := maskTestApp(aiProviderOpenAI, func(input string) (string, error) {
		sent = input
		return "MIMIR_IP_1 refuses connections", nil
	})
	output := "export OPENAI_API_KEY=sk-abcdefghijklmnopqrstuv\nconnect to 10.0.3.17 refused"

	result, err := workflowAIRunner{app: app}.Run("AI mode: explain_output", "bash", "Bash", output)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, leak := range []string{"sk-abcdefghijklmnopqrstuv", "10.0.3.17"} {
		if strings.Contains(sent, leak) {
			t.Errorf("%q left the machine: %s", leak, sent)
		}
	}
	if result != "10.0.3.17 refuses connections" {
		t.Fatalf("result = %q, want the answer translated back", result)
	}
}

func TestUnmaskSelectionTranslatesVariablesAndReportsUnknown(t *testing.T) {
	mask := newIdentifierMask(loadEffectiveAIFlowConfig().ProviderPolicy(aiProviderOpenAI))
	mask.mask("hosts 10.0.3.17 and 10.0.3.18")

	selection := toolSelection{
		Variables: map[string]string{"Host": "MIMIR_IP_2", "Port": "22", "Other": "MIMIR_IP_5"},
		Reason:    "MIMIR_IP_2 answered last",
	}
	unknown := mask.unmaskSelection(&selection)
	if selection.Variables["Host"] != "10.0.3.18" || selection.Variables["Port"] != "22" {
		t.Fatalf("variables = %v", selection.Variables)
	}
	if selection.Reason != "10.0.3.18 answered last" {
		t.Fatalf("reason = %q", selection.Reason)
	}
	if len(unknown) != 1 || unknown[0] != "MIMIR_IP_5" {
		t.Fatalf("unknown = %v, want the invented placeholder", unknown)
	}

	// A nil mask (local provider) changes nothing.
	var none *identifierMask
	if got := none.mask("10.0.3.17"); got != "10.0.3.17" {
		t.Fatalf("nil mask changed the text: %q", got)
	}
	if none.unmaskSelection(&selection) != nil {
		t.Fatal("nil mask reported unknown placeholders")
	}
}
