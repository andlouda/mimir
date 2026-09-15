package main

import (
	"encoding/json"
	"strings"
	"testing"

	"mimir/ssh"
)

func newTestSecretStore(t *testing.T) *ssh.SecretStore {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("MIMIR_SECRET_BACKEND", "file")
	store, err := ssh.NewSecretStore()
	if err != nil {
		t.Fatalf("new secret store: %v", err)
	}
	if err := store.SetupMasterPassword("correct horse battery"); err != nil {
		t.Fatalf("setup master password: %v", err)
	}
	return store
}

func decodeSettingsPayload(t *testing.T, payload string) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal([]byte(payload), &out); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	return out
}

// TestAISettingsPayloadsNeverContainTheAPIKey is the regression test for the
// key leaking into the webview: every JSON handed to the frontend must be
// redacted, while the backend keeps using the real key.
func TestAISettingsPayloadsNeverContainTheAPIKey(t *testing.T) {
	store := newTestSecretStore(t)
	app := &App{
		sshSecretStore: store,
		aiSettings:     AISettings{Provider: aiProviderOpenAI, Model: "m", BaseURL: "https://api.example/v1", APIKey: "sk-secret"},
	}

	payload, err := app.GetAISettingsJSON()
	if err != nil {
		t.Fatalf("GetAISettingsJSON: %v", err)
	}
	if strings.Contains(payload, "sk-secret") {
		t.Fatalf("GetAISettingsJSON leaked the key: %s", payload)
	}
	got := decodeSettingsPayload(t, payload)
	if got["hasApiKey"] != true {
		t.Fatalf("hasApiKey = %v, want true", got["hasApiKey"])
	}
	if v, ok := got["apiKey"]; ok && v != "" {
		t.Fatalf("apiKey should be absent or empty, got %v", v)
	}

	// Saving without a key keeps the stored one.
	payload, err = app.UpdateAISettingsJSON(`{"provider":"openai","model":"m2","baseUrl":"https://api.example/v1","apiKey":""}`)
	if err != nil {
		t.Fatalf("UpdateAISettingsJSON: %v", err)
	}
	if strings.Contains(payload, "sk-secret") {
		t.Fatalf("UpdateAISettingsJSON leaked the key: %s", payload)
	}
	if current := app.currentAISettings(); current.APIKey != "sk-secret" || current.Model != "m2" {
		t.Fatalf("backend settings = %+v, want stored key kept and model updated", current)
	}

	// A new key replaces the stored one and is persisted in the secret store.
	if _, err := app.UpdateAISettingsJSON(`{"provider":"openai","model":"m2","baseUrl":"https://api.example/v1","apiKey":"sk-new"}`); err != nil {
		t.Fatalf("UpdateAISettingsJSON (new key): %v", err)
	}
	if current := app.currentAISettings(); current.APIKey != "sk-new" {
		t.Fatalf("backend key = %q, want sk-new", current.APIKey)
	}
	if stored, err := store.GetPassword(aiAPIKeySecretID); err != nil || stored != "sk-new" {
		t.Fatalf("stored key = %q (err %v), want sk-new", stored, err)
	}

	// An explicit clear removes it everywhere.
	payload, err = app.UpdateAISettingsJSON(`{"provider":"openai","model":"m2","baseUrl":"https://api.example/v1","clearApiKey":true}`)
	if err != nil {
		t.Fatalf("UpdateAISettingsJSON (clear): %v", err)
	}
	if got := decodeSettingsPayload(t, payload); got["hasApiKey"] != false {
		t.Fatalf("hasApiKey after clear = %v, want false", got["hasApiKey"])
	}
	if current := app.currentAISettings(); current.APIKey != "" {
		t.Fatalf("backend key after clear = %q, want empty", current.APIKey)
	}
	if _, err := store.GetPassword(aiAPIKeySecretID); err == nil {
		t.Fatalf("expected stored key to be deleted")
	}
}

func TestValidateAIBaseURLRequiresHTTPSWithKey(t *testing.T) {
	cases := []struct {
		url    string
		hasKey bool
		ok     bool
	}{
		{"https://api.openai.com/v1/responses", true, true},
		{"http://localhost:11434/api/chat", true, true},
		{"http://127.0.0.1:11434/api/chat", true, true},
		{"http://[::1]:11434/api/chat", true, true},
		{"http://ollama.lan:11434/api/chat", false, true},
		{"http://ollama.lan:11434/api/chat", true, false},
		{"http://10.0.0.5:11434/api/chat", true, false},
		{"ftp://x/y", false, false},
		{"", true, true},
	}
	for _, c := range cases {
		err := validateAIBaseURL(c.url, c.hasKey)
		if (err == nil) != c.ok {
			t.Fatalf("validateAIBaseURL(%q, hasKey=%v) = %v, want ok=%v", c.url, c.hasKey, err, c.ok)
		}
	}
}
