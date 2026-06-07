package aiflow

import (
	"strings"
	"testing"
)

func TestSanitizeTerminalContextStrictProvider(t *testing.T) {
	raw := "\x1b[31mAuthorization: Bearer abcdefghijklmnopqrstuvwxyz\x1b[0m\nOPENAI_API_KEY=sk-secret-123456789012\npath ~/.ssh/id_ed25519\nsafe line"
	result := SanitizeTerminalContext(raw, ProviderPolicyConfig{
		AllowSensitiveContext: false,
		MaxContextChars:       2000,
	})

	if strings.Contains(result.Value, "Bearer") || strings.Contains(result.Value, "sk-secret") {
		t.Fatalf("expected secrets to be redacted, got %q", result.Value)
	}
	if strings.Contains(result.Value, ".ssh/id_ed25519") {
		t.Fatalf("expected sensitive paths to be removed, got %q", result.Value)
	}
	if !strings.Contains(result.Value, "safe line") {
		t.Fatalf("expected safe line to remain, got %q", result.Value)
	}
	if len(result.Redactions) == 0 {
		t.Fatalf("expected redactions to be reported")
	}
	if result.Audit.RawChars != len(raw) {
		t.Fatalf("expected raw char count %d, got %d", len(raw), result.Audit.RawChars)
	}
	if result.Audit.RedactionCounts["authorization_header"] != 1 {
		t.Fatalf("expected authorization_header count 1, got %d", result.Audit.RedactionCounts["authorization_header"])
	}
	if result.Audit.RedactionCounts["openai_api_key"] != 1 {
		t.Fatalf("expected openai_api_key count 1, got %d", result.Audit.RedactionCounts["openai_api_key"])
	}
	if result.Audit.RemovedLineCounts["secret_path"] != 1 {
		t.Fatalf("expected secret_path removed line count 1, got %d", result.Audit.RemovedLineCounts["secret_path"])
	}
	if result.Audit.SanitizedChars != len(result.Value) {
		t.Fatalf("expected sanitized char count %d, got %d", len(result.Value), result.Audit.SanitizedChars)
	}
}

func TestSanitizeTerminalContextStripsControlSequences(t *testing.T) {
	// Cursor hide/show (private mode), an OSC window-title change, a CSI color,
	// and a stray BEL — none of these should reach the model.
	raw := "\x1b[?25l\x1b]0;C:\\WINDOWS\\SYSTEM32\\bash.exe\x07\x1b[32m$ ls\x1b[0m\nfile1 file2\x07\x1b[?25h"
	result := SanitizeTerminalContext(raw, ProviderPolicyConfig{
		AllowSensitiveContext: true,
		MaxContextChars:       2000,
	})

	for _, leak := range []string{"\x1b", "[?25l", "[?25h", "\x07", "0;C:\\WINDOWS", "SYSTEM32\\bash.exe"} {
		if strings.Contains(result.Value, leak) {
			t.Fatalf("control/title leak %q survived: %q", leak, result.Value)
		}
	}
	if !strings.Contains(result.Value, "$ ls") || !strings.Contains(result.Value, "file1 file2") {
		t.Fatalf("expected visible text to remain, got %q", result.Value)
	}
}

func TestSanitizeTerminalContextRespectsMaxChars(t *testing.T) {
	raw := "1234567890ABCDEFGHIJ"
	result := SanitizeTerminalContext(raw, ProviderPolicyConfig{
		AllowSensitiveContext: true,
		MaxContextChars:       8,
	})

	if result.Value != "CDEFGHIJ" {
		t.Fatalf("expected trailing context slice, got %q", result.Value)
	}
	if !result.Audit.Truncated {
		t.Fatalf("expected audit to report truncation")
	}
	if result.Audit.MaxChars != 8 {
		t.Fatalf("expected max chars 8, got %d", result.Audit.MaxChars)
	}
}
