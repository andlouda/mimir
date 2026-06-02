package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// providerModelFromEnv / providerURLFromEnv / providerAPIKeyFromEnv preserve the
// previous environment-variable overrides for the well-known providers.
func providerModelFromEnv(id string) string {
	switch id {
	case aiProviderOpenAI:
		return strings.TrimSpace(os.Getenv("OPENAI_MODEL"))
	case aiProviderOllama:
		return strings.TrimSpace(os.Getenv("OLLAMA_MODEL"))
	case aiProviderAnthropic:
		return strings.TrimSpace(os.Getenv("ANTHROPIC_MODEL"))
	}
	return ""
}

func providerURLFromEnv(id string) string {
	if id == aiProviderOllama {
		return strings.TrimSpace(os.Getenv("OLLAMA_BASE_URL"))
	}
	return ""
}

func providerAPIKeyFromEnv(id string) string {
	switch id {
	case aiProviderOpenAI:
		return strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	case aiProviderAnthropic:
		return strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY"))
	}
	return ""
}

// AI provider protocols. The provider registry maps each provider id to one of
// these wire protocols; callAIProvider dispatches on the protocol rather than
// on a hard-coded provider id, so new providers are just data.
const (
	protocolOpenAIResponses = "openai_responses" // OpenAI /v1/responses
	protocolOpenAIChat      = "openai_chat"       // OpenAI-compatible /v1/chat/completions
	protocolOllamaChat      = "ollama_chat"       // Ollama /api/chat
	protocolAnthropicMsgs   = "anthropic_messages" // Anthropic /v1/messages

	aiProviderAnthropic        = "anthropic"
	aiProviderOpenAICompatible = "openai_compatible"

	defaultAnthropicModel  = "claude-sonnet-4-6"
	defaultAnthropicURL    = "https://api.anthropic.com/v1/messages"
	defaultOpenAICompatURL = "http://localhost:1234/v1/chat/completions"

	anthropicVersionHeader = "2023-06-01"
	anthropicMaxTokens     = 1024
)

// AIProviderDescriptor describes a selectable AI provider. The list is exposed
// to the frontend so the provider picker is data-driven (no hard-coded UI).
type AIProviderDescriptor struct {
	ID             string `json:"id"`
	Label          string `json:"label"`
	Protocol       string `json:"protocol"`
	DefaultModel   string `json:"defaultModel"`
	DefaultBaseURL string `json:"defaultBaseUrl"`
	RequiresAPIKey bool   `json:"requiresApiKey"` // a key is mandatory for this provider
	SupportsAPIKey bool   `json:"supportsApiKey"` // show the API key field (optional use)
	AllowCustomURL bool   `json:"allowCustomUrl"` // base URL is meant to be edited
}

// aiProviderDescriptors is the built-in provider registry. Presets cover the
// dedicated protocols; "openai_compatible" is a generic chat-completions client
// that works with LM Studio, vLLM, OpenRouter, Groq and similar endpoints.
func aiProviderDescriptors() []AIProviderDescriptor {
	return []AIProviderDescriptor{
		{
			ID:             aiProviderOpenAI,
			Label:          "OpenAI API",
			Protocol:       protocolOpenAIResponses,
			DefaultModel:   defaultOpenAIModel,
			DefaultBaseURL: defaultOpenAIURL,
			RequiresAPIKey: true,
			SupportsAPIKey: true,
		},
		{
			ID:             aiProviderAnthropic,
			Label:          "Anthropic (Claude)",
			Protocol:       protocolAnthropicMsgs,
			DefaultModel:   defaultAnthropicModel,
			DefaultBaseURL: defaultAnthropicURL,
			RequiresAPIKey: true,
			SupportsAPIKey: true,
		},
		{
			ID:             aiProviderOllama,
			Label:          "Ollama (Local)",
			Protocol:       protocolOllamaChat,
			DefaultModel:   defaultOllamaModel,
			DefaultBaseURL: defaultOllamaURL,
			RequiresAPIKey: false,
			SupportsAPIKey: true, // remote/proxied Ollama may sit behind auth
			AllowCustomURL: true,
		},
		{
			ID:             aiProviderOpenAICompatible,
			Label:          "OpenAI-compatible (Custom)",
			Protocol:       protocolOpenAIChat,
			DefaultModel:   "",
			DefaultBaseURL: defaultOpenAICompatURL,
			RequiresAPIKey: false,
			SupportsAPIKey: true,
			AllowCustomURL: true,
		},
	}
}

// providerDescriptor returns the descriptor for id, falling back to the first
// (OpenAI) descriptor for unknown ids.
func providerDescriptor(id string) AIProviderDescriptor {
	descriptors := aiProviderDescriptors()
	for _, d := range descriptors {
		if d.ID == id {
			return d
		}
	}
	return descriptors[0]
}

// GetAIProvidersJSON returns the provider registry for the frontend picker.
func (a *App) GetAIProvidersJSON() (string, error) {
	payload, err := json.Marshal(aiProviderDescriptors())
	if err != nil {
		return "", fmt.Errorf("failed to encode AI providers: %w", err)
	}
	return string(payload), nil
}

// --- Anthropic Messages API ---

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (a *App) callAnthropic(settings AISettings, input string) (string, error) {
	if strings.TrimSpace(settings.APIKey) == "" {
		return "", fmt.Errorf("API key is not configured")
	}

	reqBody := anthropicRequest{
		Model:     settings.Model,
		MaxTokens: anthropicMaxTokens,
		Messages:  []anthropicMessage{{Role: "user", Content: input}},
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to encode Anthropic request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, settings.BaseURL, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("failed to create Anthropic request: %w", err)
	}
	req.Header.Set("x-api-key", settings.APIKey)
	req.Header.Set("anthropic-version", anthropicVersionHeader)
	req.Header.Set("content-type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("Anthropic request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read Anthropic response: %w", err)
	}

	var parsed anthropicResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("failed to decode Anthropic response: %w", err)
	}

	if resp.StatusCode >= 400 {
		if parsed.Error != nil && strings.TrimSpace(parsed.Error.Message) != "" {
			return "", fmt.Errorf("Anthropic API error: %s", parsed.Error.Message)
		}
		return "", fmt.Errorf("Anthropic API error: status %d", resp.StatusCode)
	}

	var builder strings.Builder
	for _, block := range parsed.Content {
		if block.Type == "text" && strings.TrimSpace(block.Text) != "" {
			if builder.Len() > 0 {
				builder.WriteString("\n")
			}
			builder.WriteString(block.Text)
		}
	}
	text := strings.TrimSpace(builder.String())
	if text == "" {
		return "", fmt.Errorf("Anthropic returned no text output")
	}
	return text, nil
}

// --- OpenAI-compatible Chat Completions API ---

type chatCompletionsRequest struct {
	Model    string            `json:"model"`
	Messages []chatMessageBody `json:"messages"`
}

type chatMessageBody struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionsResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (a *App) callOpenAIChatCompletions(settings AISettings, input string) (string, error) {
	reqBody := chatCompletionsRequest{
		Model:    settings.Model,
		Messages: []chatMessageBody{{Role: "user", Content: input}},
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to encode chat request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, settings.BaseURL, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("failed to create chat request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if key := strings.TrimSpace(settings.APIKey); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}

	client := &http.Client{Timeout: 90 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("chat request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read chat response: %w", err)
	}

	var parsed chatCompletionsResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("failed to decode chat response: %w", err)
	}

	if resp.StatusCode >= 400 {
		if parsed.Error != nil && strings.TrimSpace(parsed.Error.Message) != "" {
			return "", fmt.Errorf("AI API error: %s", parsed.Error.Message)
		}
		return "", fmt.Errorf("AI API error: status %d", resp.StatusCode)
	}

	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("AI returned no choices")
	}
	text := strings.TrimSpace(parsed.Choices[0].Message.Content)
	if text == "" {
		return "", fmt.Errorf("AI returned no text output")
	}
	return text, nil
}
