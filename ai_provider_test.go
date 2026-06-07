package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCallAIProviderUnknownProviderFallsBackToOpenAIResponses(t *testing.T) {
	var gotAuth string
	var gotRequest responsesRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/responses" {
			t.Fatalf("path = %q, want /responses", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotRequest); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = w.Write([]byte(`{"output_text":"fallback ok"}`))
	}))
	t.Cleanup(server.Close)

	app := &App{}
	result, err := app.callAIProvider(AISettings{
		Provider: "does-not-exist",
		Model:    "fallback-model",
		BaseURL:  server.URL + "/responses",
		APIKey:   "test-key",
	}, "explain this")
	if err != nil {
		t.Fatalf("callAIProvider: %v", err)
	}
	if result != "fallback ok" {
		t.Fatalf("result = %q, want fallback ok", result)
	}
	if gotRequest.Model != "fallback-model" || gotRequest.Input != "explain this" {
		t.Fatalf("request = %#v, want model/input preserved", gotRequest)
	}
	if gotAuth != "Bearer test-key" {
		t.Fatalf("Authorization = %q, want bearer test key", gotAuth)
	}
}

func TestAskAIUsesProviderFallbackAndReturnsJSONPayload(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	app := &App{
		aiSettings: AISettings{Provider: "unknown-provider", Model: "m", BaseURL: "unused", APIKey: "k"},
		aiProviderCaller: func(settings AISettings, input string) (string, error) {
			if settings.Provider != aiProviderOpenAI {
				t.Fatalf("settings.Provider = %q, want normalized OpenAI fallback", settings.Provider)
			}
			if !strings.Contains(input, "Recent terminal output:") {
				t.Fatalf("prompt did not include terminal output context: %q", input)
			}
			return "ls -la", nil
		},
	}

	payload, err := app.AskAI("suggest_next_command", "", "bash", "Bash", "total 0")
	if err != nil {
		t.Fatalf("AskAI: %v", err)
	}
	var parsed aiAskResult
	if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
		t.Fatalf("decode AskAI payload: %v", err)
	}
	if parsed.Text != "ls -la" || parsed.Warning != "" {
		t.Fatalf("payload = %#v, want text without warning", parsed)
	}
}
