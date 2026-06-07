package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"mimir/activitylog"
	"mimir/aiflow"
	"mimir/safeio"
	mimirssh "mimir/ssh"
	"mimir/template"
	"mimir/tools"
	"mimir/workflow"
)

const (
	defaultOpenAIModel = "gpt-5.4-mini"
	defaultOllamaModel = "qwen2.5-coder:7b"
	defaultOpenAIURL   = "https://api.openai.com/v1/responses"
	defaultOllamaURL   = "http://localhost:11434/api/chat"
	maxTerminalContext = 12000
	aiProviderOpenAI   = "openai"
	aiProviderOllama   = "ollama"
	aiSettingsFileName = "ai_settings.json"
	aiAPIKeySecretID   = "__mimir_openai_api_key__"
)

type AISettings struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	BaseURL  string `json:"baseUrl"`
	APIKey   string `json:"apiKey"`
}

type responsesRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

type responsesResponse struct {
	OutputText string `json:"output_text"`
	Output     []struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"output"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

type ollamaChatRequest struct {
	Model    string `json:"model"`
	Stream   bool   `json:"stream"`
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
}

type ollamaChatResponse struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
	Error string `json:"error"`
}

type toolSelection struct {
	ToolID    string            `json:"toolId"`
	Template  string            `json:"template"`
	Variables map[string]string `json:"variables"`
	Reason    string            `json:"reason"`
}

type AIInteractionLogEntry struct {
	Timestamp         string            `json:"timestamp"`
	Provider          string            `json:"provider"`
	Model             string            `json:"model"`
	BaseURL           string            `json:"baseUrl"`
	Mode              string            `json:"mode"`
	TerminalID        int               `json:"terminalId,omitempty"`
	TerminalType      string            `json:"terminalType,omitempty"`
	TerminalName      string            `json:"terminalName,omitempty"`
	Goal              string            `json:"goal,omitempty"`
	Prompt            string            `json:"prompt"`
	TerminalOutput    string            `json:"terminalOutput,omitempty"`
	Response          string            `json:"response,omitempty"`
	ContextRedactions []string          `json:"contextRedactions,omitempty"`
	ContextAudit      map[string]any    `json:"contextAudit,omitempty"`
	Error             string            `json:"error,omitempty"`
	Template          string            `json:"template,omitempty"`
	Variables         map[string]string `json:"variables,omitempty"`
	Reason            string            `json:"reason,omitempty"`
}

func aiContextAudit(provider string, contextIncluded bool, audit aiflow.SanitizationAudit) map[string]any {
	result := map[string]any{
		"provider":        provider,
		"contextIncluded": contextIncluded,
	}
	if !contextIncluded {
		return result
	}
	result["rawChars"] = audit.RawChars
	result["normalizedChars"] = audit.NormalizedChars
	result["sanitizedChars"] = audit.SanitizedChars
	result["maxChars"] = audit.MaxChars
	result["truncated"] = audit.Truncated
	result["allowSensitiveContext"] = audit.AllowSensitiveContext
	if len(audit.RedactionCounts) > 0 {
		result["redactionCounts"] = audit.RedactionCounts
	}
	if len(audit.RemovedLineCounts) > 0 {
		result["removedLineCounts"] = audit.RemovedLineCounts
	}
	return result
}

type FunctionCatalogEntry struct {
	ID            string                       `json:"id"`
	Name          string                       `json:"name"`
	Kind          string                       `json:"kind"`
	Category      string                       `json:"category"`
	Description   string                       `json:"description"`
	Parameters    []template.TemplateParameter `json:"parameters,omitempty"`
	Commands      map[string]string            `json:"commands,omitempty"`
	DiscoveryTool string                       `json:"discoveryTool,omitempty"`
}

func normalizeAISettings(settings AISettings) AISettings {
	settings.Provider = strings.TrimSpace(settings.Provider)
	settings.Model = strings.TrimSpace(settings.Model)
	settings.BaseURL = strings.TrimSpace(settings.BaseURL)
	settings.APIKey = strings.TrimSpace(settings.APIKey)

	if settings.Provider == "" {
		if envProvider := strings.TrimSpace(os.Getenv("AI_PROVIDER")); envProvider != "" {
			settings.Provider = envProvider
		} else {
			settings.Provider = aiProviderOpenAI
		}
	}

	// Resolve against the provider registry; unknown ids fall back to OpenAI.
	desc := providerDescriptor(settings.Provider)
	settings.Provider = desc.ID

	if settings.Model == "" {
		settings.Model = firstNonEmpty(providerModelFromEnv(desc.ID), desc.DefaultModel)
	}
	if settings.BaseURL == "" {
		settings.BaseURL = firstNonEmpty(providerURLFromEnv(desc.ID), desc.DefaultBaseURL)
	}
	if settings.APIKey == "" {
		settings.APIKey = providerAPIKeyFromEnv(desc.ID)
	}

	return settings
}

func getAISettingsFilePath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user config directory: %w", err)
	}

	appConfigDir := filepath.Join(configDir, "mimir")
	if err := os.MkdirAll(appConfigDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create application config directory: %w", err)
	}

	return filepath.Join(appConfigDir, aiSettingsFileName), nil
}

func LoadAISettings(store *mimirssh.SecretStore) (AISettings, error) {
	filePath, err := getAISettingsFilePath()
	if err != nil {
		return AISettings{}, err
	}

	raw, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			settings := normalizeAISettings(AISettings{})
			if settings.Provider == aiProviderOpenAI && strings.TrimSpace(settings.APIKey) == "" {
				if apiKey, keyErr := loadStoredAIAPIKey(store); keyErr == nil {
					settings.APIKey = apiKey
				}
			}
			return settings, nil
		}
		return AISettings{}, fmt.Errorf("failed to read AI settings: %w", err)
	}

	var settings AISettings
	if err := json.Unmarshal(raw, &settings); err != nil {
		return AISettings{}, fmt.Errorf("failed to parse AI settings: %w", err)
	}

	settings = normalizeAISettings(settings)
	if settings.Provider == aiProviderOpenAI && strings.TrimSpace(settings.APIKey) == "" {
		if apiKey, err := loadStoredAIAPIKey(store); err == nil {
			settings.APIKey = apiKey
		}
	}
	if settings.Provider == aiProviderOpenAI && strings.TrimSpace(settings.APIKey) != "" {
		_ = storeAIAPIKey(store, settings.APIKey)
		withoutSecret := settings
		withoutSecret.APIKey = ""
		if payload, err := json.MarshalIndent(withoutSecret, "", "  "); err == nil {
			_ = safeio.AtomicWriteFile(filePath, payload, 0600)
		}
	}

	return settings, nil
}

func SaveAISettings(store *mimirssh.SecretStore, settings AISettings) (AISettings, error) {
	filePath, err := getAISettingsFilePath()
	if err != nil {
		return AISettings{}, err
	}

	settings = normalizeAISettings(settings)
	if settings.Provider == aiProviderOpenAI && strings.TrimSpace(settings.APIKey) != "" {
		if err := storeAIAPIKey(store, settings.APIKey); err != nil {
			return AISettings{}, err
		}
	}

	settingsForFile := settings
	settingsForFile.APIKey = ""
	payload, err := json.MarshalIndent(settingsForFile, "", "  ")
	if err != nil {
		return AISettings{}, fmt.Errorf("failed to encode AI settings: %w", err)
	}

	if err := safeio.AtomicWriteFile(filePath, payload, 0600); err != nil {
		return AISettings{}, fmt.Errorf("failed to write AI settings: %w", err)
	}

	return settings, nil
}

func storeAIAPIKey(store *mimirssh.SecretStore, apiKey string) error {
	if store == nil {
		return fmt.Errorf("secret store unavailable")
	}
	return store.SetPassword(aiAPIKeySecretID, apiKey)
}

func loadStoredAIAPIKey(store *mimirssh.SecretStore) (string, error) {
	if store == nil {
		return "", fmt.Errorf("secret store unavailable")
	}
	return store.GetPassword(aiAPIKeySecretID)
}

func trimContext(input string, maxLen int) string {
	if len(input) <= maxLen {
		return input
	}
	return input[len(input)-maxLen:]
}

func (a *App) invalidateFunctionCatalog() {
	a.functionCatalogMu.Lock()
	defer a.functionCatalogMu.Unlock()
	a.functionCatalog = nil
	a.functionCatalogJSON = ""
}

func (a *App) buildFunctionCatalog() []FunctionCatalogEntry {
	entries := []FunctionCatalogEntry{
		{
			ID:          "ai:explain_output",
			Name:        "Explain Output",
			Kind:        "ai_action",
			Category:    "AI",
			Description: "Explains the recent terminal output and what it means.",
		},
		{
			ID:          "ai:suggest_next_command",
			Name:        "Suggest Next Command",
			Kind:        "ai_action",
			Category:    "AI",
			Description: "Suggests exactly one next command based on the active terminal output.",
		},
		{
			ID:          "ai:write_command_from_goal",
			Name:        "Write Command From Goal",
			Kind:        "ai_action",
			Category:    "AI",
			Description: "Generates exactly one command from a natural language goal.",
			Parameters: []template.TemplateParameter{
				{Name: "Goal", Required: true},
			},
		},
		{
			ID:          "ai:run_template_tool",
			Name:        "Run Template Tool From Goal",
			Kind:        "ai_action",
			Category:    "AI",
			Description: "Lets the AI choose and execute the best matching runnable template tool for a goal.",
			Parameters: []template.TemplateParameter{
				{Name: "Goal", Required: true},
			},
		},
	}

	entries = append(entries, discoveryFunctionCatalogEntries()...)

	for _, tmpl := range a.TemplateManager.GetToolTemplates() {
		entries = append(entries, FunctionCatalogEntry{
			ID:          "template:" + tmpl.Name,
			Name:        tmpl.Name,
			Kind:        "template_tool",
			Category:    tmpl.Category,
			Description: tmpl.Description,
			Parameters:  tmpl.Parameters,
			Commands:    tmpl.Commands,
		})
	}

	return entries
}

func discoveryFunctionCatalogEntries() []FunctionCatalogEntry {
	return []FunctionCatalogEntry{
		{
			ID:            "discovery:list_k8s_namespaces",
			Name:          "Discovery: K8s Namespaces",
			Kind:          "discovery_tool",
			Category:      "Discovery",
			Description:   "Lists Kubernetes namespaces from the current cluster context.",
			DiscoveryTool: "discovery:list_k8s_namespaces",
		},
		{
			ID:            "discovery:list_k8s_pods",
			Name:          "Discovery: K8s Pods",
			Kind:          "discovery_tool",
			Category:      "Discovery",
			Description:   "Lists pods in a namespace from the current cluster context.",
			DiscoveryTool: "discovery:list_k8s_pods",
			Parameters: []template.TemplateParameter{
				{Name: "Namespace", Required: true, Pattern: "^[a-z0-9]([-a-z0-9]*[a-z0-9])?$", MaxLength: 63},
			},
		},
		{
			ID:            "discovery:list_docker_containers",
			Name:          "Discovery: Docker Containers",
			Kind:          "discovery_tool",
			Category:      "Discovery",
			Description:   "Lists currently running Docker container names.",
			DiscoveryTool: "discovery:list_docker_containers",
		},
		{
			ID:            "discovery:list_compose_services",
			Name:          "Discovery: Compose Services",
			Kind:          "discovery_tool",
			Category:      "Discovery",
			Description:   "Lists Docker Compose services in the current project directory.",
			DiscoveryTool: "discovery:list_compose_services",
		},
		{
			ID:            "discovery:list_k8s_resources",
			Name:          "Discovery: K8s Resources",
			Kind:          "discovery_tool",
			Category:      "Discovery",
			Description:   "Lists resource names for a Kubernetes resource type inside a namespace.",
			DiscoveryTool: "discovery:list_k8s_resources",
			Parameters: []template.TemplateParameter{
				{Name: "Namespace", Required: true, Pattern: "^[a-z0-9]([-a-z0-9]*[a-z0-9])?$", MaxLength: 63},
				{Name: "ResourceType", Required: true, Options: []string{"pod", "deployment", "service", "ingress", "configmap", "secret"}},
			},
		},
	}
}

func (a *App) getFunctionCatalog() ([]FunctionCatalogEntry, error) {
	a.functionCatalogMu.Lock()
	defer a.functionCatalogMu.Unlock()

	if len(a.functionCatalog) > 0 {
		return a.functionCatalog, nil
	}

	entries := a.buildFunctionCatalog()
	payload, err := json.Marshal(entries)
	if err != nil {
		return nil, fmt.Errorf("failed to encode function catalog: %w", err)
	}

	a.functionCatalog = entries
	a.functionCatalogJSON = string(payload)
	return a.functionCatalog, nil
}

func (a *App) logAIInteraction(entry AIInteractionLogEntry) {
	entry.Timestamp = time.Now().Format(time.RFC3339)
	_ = activitylog.Append(activitylog.KindAIInteractions, entry)
}

func buildAIPrompt(mode string, goal string, terminalType string, terminalName string, terminalOutput string) (string, error) {
	baseContext := fmt.Sprintf(
		"Terminal type: %s\nTerminal name: %s\nRecent terminal output:\n%s\n",
		terminalType,
		terminalName,
		terminalOutput,
	)

	switch mode {
	case "explain_output":
		return "You are a terminal assistant. Explain the recent terminal output concisely and practically. Focus on what happened, whether it indicates an error, and the most relevant next interpretation. Do not use markdown code fences.\n\n" + baseContext, nil
	case "suggest_next_command":
		return "You are a terminal assistant. Based on the recent terminal output, return exactly one next shell command for the given terminal type. Return only the command, with no explanation, no markdown, and no surrounding quotes.\n\n" + baseContext, nil
	case "write_command_from_goal":
		if strings.TrimSpace(goal) == "" {
			return "", fmt.Errorf("goal is required")
		}
		return fmt.Sprintf(
			"You are a terminal assistant. Write exactly one shell command for the given terminal type that best accomplishes the user's goal. Return only the command, with no explanation, no markdown, and no surrounding quotes.\n\n%s\nUser goal:\n%s\n",
			baseContext,
			goal,
		), nil
	default:
		return "", fmt.Errorf("unsupported AI mode: %s", mode)
	}
}

func extractResponseText(resp responsesResponse) string {
	if strings.TrimSpace(resp.OutputText) != "" {
		return strings.TrimSpace(resp.OutputText)
	}

	var builder strings.Builder
	for _, item := range resp.Output {
		for _, content := range item.Content {
			if strings.TrimSpace(content.Text) == "" {
				continue
			}
			if builder.Len() > 0 {
				builder.WriteString("\n")
			}
			builder.WriteString(content.Text)
		}
	}
	return strings.TrimSpace(builder.String())
}

func extractJSONObject(input string) string {
	start := strings.Index(input, "{")
	end := strings.LastIndex(input, "}")
	if start == -1 || end == -1 || end < start {
		return ""
	}
	return input[start : end+1]
}

func buildToolSelectionPrompt(goal string, terminalType string, terminalName string, terminalOutput string, availableTools []tools.Tool, config aiflow.Config) (string, error) {
	if strings.TrimSpace(goal) == "" {
		return "", fmt.Errorf("goal is required")
	}

	toolSummaries := make([]map[string]any, 0, len(availableTools))
	for _, tool := range availableTools {
		params := make([]map[string]any, 0, len(tool.Parameters()))
		for _, param := range tool.Parameters() {
			params = append(params, map[string]any{
				"name":        param.Name,
				"description": param.Description,
				"required":    param.Required,
			})
		}
		toolSummary := map[string]any{
			"id":          tool.ID(),
			"name":        tool.Name(),
			"description": tool.Description(),
			"parameters":  params,
			"class":       string(tool.Class()),
		}
		if config.Prompt.IncludeCategory {
			toolSummary["category"] = tool.Category()
		}
		if config.Prompt.IncludeRisk {
			toolSummary["risk"] = string(tool.Risk())
		}
		toolSummaries = append(toolSummaries, toolSummary)
	}

	toolJSON, err := json.MarshalIndent(toolSummaries, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to encode tool metadata: %w", err)
	}

	trimmedOutput := ""
	if config.Prompt.IncludeTerminalOutput {
		trimmedOutput = strings.TrimSpace(terminalOutput)
	}
	selectionShape := `{"toolId":"...","variables":{"Param":"value"},"reason":"..."}`
	if !config.Prompt.RequireStableToolID && config.Prompt.AllowTemplateNameFallback {
		selectionShape = `{"toolId":"...","template":"...","variables":{"Param":"value"},"reason":"..."}`
	}
	stableIDInstruction := "Use only the provided tools and refer to them by the exact stable tool id."
	if !config.Prompt.RequireStableToolID {
		stableIDInstruction = "Use only the provided tools. Prefer the exact stable tool id."
	}
	intro := strings.TrimSpace(config.Prompt.PrePrompt)
	if intro == "" {
		intro = "You are a terminal automation assistant."
	}
	return fmt.Sprintf(
		"%s\n\n%s\n\nChoose the single best registered tool to execute for the user's goal. %s Fill required parameters when possible. If no tool fits, return an empty toolId.\nReturn strictly valid JSON with this shape and nothing else:\n%s\n\nTerminal type: %s\nTerminal name: %s\nRecent terminal output:\n%s\n\nUser goal:\n%s\n\nAvailable tools:\n%s\n",
		intro,
		aiflow.ImmutablePromptGuardrails(),
		stableIDInstruction,
		selectionShape,
		terminalType,
		terminalName,
		trimmedOutput,
		goal,
		string(toolJSON),
	), nil
}

func loadEffectiveAIFlowConfig() aiflow.Config {
	config, err := aiflow.LoadConfig()
	if err != nil {
		config = aiflow.DefaultConfig()
	}
	return aiflow.ApplyImmutableGuardrails(config)
}

func sanitizeTerminalOutputForAI(provider string, terminalOutput string) aiflow.SanitizedContext {
	config := loadEffectiveAIFlowConfig()
	return aiflow.SanitizeTerminalContext(terminalOutput, config.ProviderPolicy(provider))
}

func buildAIToolRegistry(manager *template.Manager) (*tools.Registry, []tools.Tool, error) {
	if manager == nil {
		return nil, nil, fmt.Errorf("template manager is required")
	}

	registry := tools.NewRegistry()
	toolTemplates := manager.GetToolTemplates()
	filteredTemplates := make([]template.Template, 0, len(toolTemplates))
	for _, tmpl := range toolTemplates {
		if aiflow.TemplateAllowedForAI(tmpl) {
			filteredTemplates = append(filteredTemplates, tmpl)
		}
	}
	templateTools := tools.BuildTemplateTools(manager, filteredTemplates)
	for _, tool := range templateTools {
		if err := registry.Register(tool); err != nil {
			return nil, nil, err
		}
	}

	registeredTools := registry.List()
	return registry, registeredTools, nil
}

func resolveSelectedTool(registry *tools.Registry, selection toolSelection, config aiflow.Config) (tools.Tool, error) {
	if registry == nil {
		return nil, fmt.Errorf("tool registry is required")
	}

	if toolID := strings.TrimSpace(selection.ToolID); toolID != "" {
		if tool, ok := registry.Get(toolID); ok {
			return tool, nil
		}
		return nil, fmt.Errorf("AI selected unknown tool id %s", toolID)
	}

	templateName := strings.TrimSpace(selection.Template)
	if templateName == "" || !config.Prompt.AllowTemplateNameFallback {
		return nil, fmt.Errorf("AI did not select a runnable tool")
	}

	for _, tool := range registry.List() {
		if tool.Name() == templateName {
			return tool, nil
		}
	}

	return nil, fmt.Errorf("AI selected unknown tool name %s", templateName)
}

func sortedStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return map[string]string{}
	}

	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	result := make(map[string]string, len(input))
	for _, key := range keys {
		result[key] = input[key]
	}
	return result
}

func buildFunctionExplanationPrompt(entry FunctionCatalogEntry, question string) (string, error) {
	entryJSON, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to encode function entry: %w", err)
	}

	userQuestion := strings.TrimSpace(question)
	if userQuestion == "" {
		userQuestion = "What does this function do, when should it be used, and what should the user expect?"
	}

	return fmt.Sprintf(
		"You are an assistant for the Mimir terminal app. Explain the following function clearly and practically. Cover what it does, when to use it, what parameters matter, and any important caveats. If commands are present, briefly explain them. Do not use markdown code fences.\n\nFunction:\n%s\n\nUser question:\n%s\n",
		string(entryJSON),
		userQuestion,
	), nil
}

func (a *App) GetAIToolFlowConfigJSON() (string, error) {
	config, err := aiflow.LoadConfig()
	if err != nil {
		return "", err
	}
	config = aiflow.ApplyImmutableGuardrails(config)

	payload, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("failed to encode AI tool flow config: %w", err)
	}

	return string(payload), nil
}

func (a *App) UpdateAIToolFlowConfigJSON(configJSON string) (string, error) {
	var config aiflow.Config
	if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
		return "", fmt.Errorf("failed to parse AI tool flow config: %w", err)
	}
	config = aiflow.ApplyImmutableGuardrails(config)

	saved, err := aiflow.SaveConfig(config)
	if err != nil {
		return "", err
	}

	payload, err := json.Marshal(saved)
	if err != nil {
		return "", fmt.Errorf("failed to encode AI tool flow config: %w", err)
	}

	return string(payload), nil
}

func (a *App) GetAISettingsJSON() (string, error) {
	a.aiMu.Lock()
	settings := normalizeAISettings(a.aiSettings)
	a.aiMu.Unlock()

	payload, err := json.Marshal(settings)
	if err != nil {
		return "", fmt.Errorf("failed to encode AI settings: %w", err)
	}

	return string(payload), nil
}

func validateAIBaseURL(rawURL string) error {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid AI base URL: %w", err)
	}
	switch parsed.Scheme {
	case "http", "https":
	default:
		return fmt.Errorf("AI base URL must use http or https scheme")
	}
	if parsed.Host == "" {
		return fmt.Errorf("AI base URL must include a host")
	}
	return nil
}

func (a *App) UpdateAISettingsJSON(settingsJSON string) (string, error) {
	var settings AISettings
	if err := json.Unmarshal([]byte(settingsJSON), &settings); err != nil {
		return "", fmt.Errorf("failed to parse AI settings: %w", err)
	}
	if err := validateAIBaseURL(settings.BaseURL); err != nil {
		return "", err
	}

	saved, err := SaveAISettings(a.sshSecretStore, settings)
	if err != nil {
		return "", err
	}

	a.aiMu.Lock()
	a.aiSettings = saved
	a.aiMu.Unlock()

	payload, err := json.Marshal(saved)
	if err != nil {
		return "", fmt.Errorf("failed to encode AI settings: %w", err)
	}

	return string(payload), nil
}

func (a *App) currentAISettings() AISettings {
	a.aiMu.Lock()
	defer a.aiMu.Unlock()
	return normalizeAISettings(a.aiSettings)
}

func (a *App) callOpenAI(settings AISettings, input string) (string, error) {
	if strings.TrimSpace(settings.APIKey) == "" {
		return "", fmt.Errorf("API key is not configured")
	}

	reqBody := responsesRequest{
		Model: settings.Model,
		Input: input,
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to encode AI request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, settings.BaseURL, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("failed to create AI request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+settings.APIKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 45 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("AI request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read AI response: %w", err)
	}

	var parsed responsesResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("failed to decode AI response: %w", err)
	}

	if resp.StatusCode >= 400 {
		if parsed.Error != nil && strings.TrimSpace(parsed.Error.Message) != "" {
			return "", fmt.Errorf("AI API error: %s", parsed.Error.Message)
		}
		return "", fmt.Errorf("AI API error: status %d", resp.StatusCode)
	}

	text := extractResponseText(parsed)
	if text == "" {
		return "", fmt.Errorf("AI returned no text output")
	}

	return text, nil
}

func (a *App) callOllama(settings AISettings, input string) (string, error) {
	reqBody := ollamaChatRequest{
		Model:  settings.Model,
		Stream: false,
	}
	reqBody.Messages = append(reqBody.Messages, struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}{
		Role:    "user",
		Content: input,
	})

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to encode Ollama request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, settings.BaseURL, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("failed to create Ollama request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	// Optional: a remote/proxied Ollama (or OpenAI-compatible local server) may
	// sit behind auth, so forward the API key as a bearer token when set.
	if key := strings.TrimSpace(settings.APIKey); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}

	client := &http.Client{Timeout: 300 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("Ollama request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read Ollama response: %w", err)
	}

	var parsed ollamaChatResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("failed to decode Ollama response: %w", err)
	}

	if resp.StatusCode >= 400 {
		if strings.TrimSpace(parsed.Error) != "" {
			return "", fmt.Errorf("Ollama error: %s", parsed.Error)
		}
		return "", fmt.Errorf("Ollama error: status %d", resp.StatusCode)
	}

	text := strings.TrimSpace(parsed.Message.Content)
	if text == "" {
		return "", fmt.Errorf("Ollama returned no text output")
	}

	return text, nil
}

func (a *App) callAIProvider(settings AISettings, input string) (string, error) {
	if a.aiProviderCaller != nil {
		return a.aiProviderCaller(settings, input)
	}
	switch providerDescriptor(settings.Provider).Protocol {
	case protocolOllamaChat:
		return a.callOllama(settings, input)
	case protocolAnthropicMsgs:
		return a.callAnthropic(settings, input)
	case protocolOpenAIChat:
		return a.callOpenAIChatCompletions(settings, input)
	default:
		return a.callOpenAI(settings, input)
	}
}

// aiAskResult is the JSON payload returned by AskAI. The raw model text is
// always surfaced (so the UI can show it); Warning carries a non-fatal
// validation note (e.g. "multiple lines for single-command mode") instead of
// rejecting the call and discarding the output.
type aiAskResult struct {
	Text    string `json:"text"`
	Warning string `json:"warning,omitempty"`
}

// AskAI sends terminal context to the configured AI provider and returns the
// model output as a JSON-encoded aiAskResult (text + optional warning).
func (a *App) AskAI(mode string, goal string, terminalType string, terminalName string, terminalOutput string) (string, error) {
	settings := a.currentAISettings()
	sanitizedContext := sanitizeTerminalOutputForAI(settings.Provider, terminalOutput)
	contextAudit := aiContextAudit(settings.Provider, true, sanitizedContext.Audit)

	input, err := buildAIPrompt(mode, goal, terminalType, terminalName, sanitizedContext.Value)
	if err != nil {
		a.logAIInteraction(AIInteractionLogEntry{
			Provider:          settings.Provider,
			Model:             settings.Model,
			BaseURL:           settings.BaseURL,
			Mode:              mode,
			TerminalType:      terminalType,
			TerminalName:      terminalName,
			Goal:              goal,
			TerminalOutput:    sanitizedContext.Value,
			ContextRedactions: sanitizedContext.Redactions,
			ContextAudit:      contextAudit,
			Error:             err.Error(),
		})
		return "", err
	}

	result, providerErr := a.callAIProvider(settings, input)
	warning := ""
	if providerErr == nil {
		if validationErr := aiflow.ValidateCommandSuggestion(mode, result); validationErr != nil {
			warning = validationErr.Error()
		}
	}

	entry := AIInteractionLogEntry{
		Provider:          settings.Provider,
		Model:             settings.Model,
		BaseURL:           settings.BaseURL,
		Mode:              mode,
		TerminalType:      terminalType,
		TerminalName:      terminalName,
		Goal:              goal,
		Prompt:            input,
		TerminalOutput:    sanitizedContext.Value,
		ContextRedactions: sanitizedContext.Redactions,
		ContextAudit:      contextAudit,
		Response:          result,
	}
	if providerErr != nil {
		entry.Error = providerErr.Error()
	} else if warning != "" {
		entry.Error = warning
	}
	a.logAIInteraction(entry)

	// A real provider/transport failure still rejects; a validation issue is
	// returned as a warning alongside the raw text so the UI can show it.
	if providerErr != nil {
		return "", providerErr
	}

	payload, err := json.Marshal(aiAskResult{Text: result, Warning: warning})
	if err != nil {
		return "", fmt.Errorf("failed to encode AI result: %w", err)
	}
	return string(payload), nil
}

func (a *App) GetTemplateToolsJSON() (string, error) {
	tools := a.TemplateManager.GetToolTemplates()
	payload, err := json.Marshal(tools)
	if err != nil {
		return "", fmt.Errorf("failed to encode tool templates: %w", err)
	}
	return string(payload), nil
}

func (a *App) GetFunctionCatalogJSON() (string, error) {
	a.functionCatalogMu.Lock()
	if a.functionCatalogJSON != "" {
		payload := a.functionCatalogJSON
		a.functionCatalogMu.Unlock()
		return payload, nil
	}
	a.functionCatalogMu.Unlock()

	_, err := a.getFunctionCatalog()
	if err != nil {
		return "", err
	}

	a.functionCatalogMu.Lock()
	defer a.functionCatalogMu.Unlock()
	return a.functionCatalogJSON, nil
}

func (a *App) ExplainFunction(functionID string, question string) (string, error) {
	settings := a.currentAISettings()

	catalog, err := a.getFunctionCatalog()
	if err != nil {
		return "", err
	}

	var target *FunctionCatalogEntry
	for i := range catalog {
		if catalog[i].ID == functionID {
			target = &catalog[i]
			break
		}
	}
	if target == nil {
		return "", fmt.Errorf("function %s not found", functionID)
	}

	prompt, err := buildFunctionExplanationPrompt(*target, question)
	if err != nil {
		return "", err
	}

	result, err := a.callAIProvider(settings, prompt)
	entry := AIInteractionLogEntry{
		Provider: settings.Provider,
		Model:    settings.Model,
		BaseURL:  settings.BaseURL,
		Mode:     "explain_function",
		Goal:     strings.TrimSpace(question),
		Prompt:   prompt,
		Response: result,
		Template: target.Name,
	}
	if err != nil {
		entry.Error = err.Error()
	}
	a.logAIInteraction(entry)

	return result, err
}

func (a *App) RunDiscoveryJSON(discoveryTool string, terminalType string, variablesJSON string) (string, error) {
	var variables map[string]string
	if strings.TrimSpace(variablesJSON) != "" {
		if err := json.Unmarshal([]byte(variablesJSON), &variables); err != nil {
			return "", fmt.Errorf("failed to parse discovery variables: %w", err)
		}
	}
	if variables == nil {
		variables = map[string]string{}
	}

	resolver := aiflow.NewCachedDiscoveryResolver(a.getTemplateContext().CurrentDir)
	values, err := resolver.Resolve(discoveryTool, terminalType, variables)
	if err != nil {
		return "", err
	}

	payload, err := json.Marshal(values)
	if err != nil {
		return "", fmt.Errorf("failed to encode discovery result: %w", err)
	}
	return string(payload), nil
}

func (a *App) RunAITemplateTool(id int, goal string, terminalType string, terminalName string, terminalOutput string) (string, error) {
	p, ok := a.TerminalManager.GetPty(id)
	if !ok {
		return "", fmt.Errorf("terminal with id %d not found", id)
	}

	registry, availableTools, err := buildAIToolRegistry(a.TemplateManager)
	if err != nil {
		return "", err
	}
	config, err := aiflow.LoadConfig()
	if err != nil {
		return "", err
	}
	config = aiflow.ApplyImmutableGuardrails(config)
	settings := a.currentAISettings()
	providerPolicy := config.ProviderPolicy(settings.Provider)
	if !config.Execution.Enabled {
		return "", fmt.Errorf("AI tool execution is disabled by configuration")
	}
	availableTools = config.FilterToolsForProvider(availableTools, settings.Provider)
	if len(availableTools) == 0 {
		return "", fmt.Errorf("no AI-runnable template tools available")
	}
	registry = tools.NewRegistry()
	for _, tool := range availableTools {
		if err := registry.Register(tool); err != nil {
			return "", err
		}
	}

	toolPromptTerminalOutput := ""
	var loggedTerminalOutput string
	var loggedContextRedactions []string
	contextAudit := aiContextAudit(settings.Provider, false, aiflow.SanitizationAudit{})
	if config.Prompt.IncludeTerminalOutput {
		sanitizedContext := aiflow.SanitizeTerminalContext(terminalOutput, providerPolicy)
		toolPromptTerminalOutput = sanitizedContext.Value
		loggedTerminalOutput = sanitizedContext.Value
		loggedContextRedactions = sanitizedContext.Redactions
		contextAudit = aiContextAudit(settings.Provider, true, sanitizedContext.Audit)
	}

	prompt, err := buildToolSelectionPrompt(goal, terminalType, terminalName, toolPromptTerminalOutput, availableTools, config)
	if err != nil {
		a.logAIInteraction(AIInteractionLogEntry{
			Mode:              "run_template_tool",
			TerminalID:        id,
			TerminalType:      terminalType,
			TerminalName:      terminalName,
			Goal:              goal,
			TerminalOutput:    loggedTerminalOutput,
			ContextRedactions: loggedContextRedactions,
			ContextAudit:      contextAudit,
			Error:             err.Error(),
		})
		return "", err
	}

	result, err := a.callAIProvider(settings, prompt)
	if err != nil {
		a.logAIInteraction(AIInteractionLogEntry{
			Provider:          settings.Provider,
			Model:             settings.Model,
			BaseURL:           settings.BaseURL,
			Mode:              "run_template_tool",
			TerminalID:        id,
			TerminalType:      terminalType,
			TerminalName:      terminalName,
			Goal:              goal,
			Prompt:            prompt,
			TerminalOutput:    loggedTerminalOutput,
			ContextRedactions: loggedContextRedactions,
			ContextAudit:      contextAudit,
			Error:             err.Error(),
		})
		return "", err
	}

	jsonBlock := extractJSONObject(result)
	if jsonBlock == "" {
		a.logAIInteraction(AIInteractionLogEntry{
			Provider:          settings.Provider,
			Model:             settings.Model,
			BaseURL:           settings.BaseURL,
			Mode:              "run_template_tool",
			TerminalID:        id,
			TerminalType:      terminalType,
			TerminalName:      terminalName,
			Goal:              goal,
			Prompt:            prompt,
			TerminalOutput:    loggedTerminalOutput,
			ContextRedactions: loggedContextRedactions,
			ContextAudit:      contextAudit,
			Response:          result,
			Error:             "AI returned no valid tool selection JSON",
		})
		return "", fmt.Errorf("AI returned no valid tool selection JSON")
	}

	var selection toolSelection
	if err := json.Unmarshal([]byte(jsonBlock), &selection); err != nil {
		a.logAIInteraction(AIInteractionLogEntry{
			Provider:          settings.Provider,
			Model:             settings.Model,
			BaseURL:           settings.BaseURL,
			Mode:              "run_template_tool",
			TerminalID:        id,
			TerminalType:      terminalType,
			TerminalName:      terminalName,
			Goal:              goal,
			Prompt:            prompt,
			TerminalOutput:    loggedTerminalOutput,
			ContextRedactions: loggedContextRedactions,
			ContextAudit:      contextAudit,
			Response:          result,
			Error:             fmt.Sprintf("failed to parse tool selection: %v", err),
		})
		return "", fmt.Errorf("failed to parse tool selection: %w", err)
	}

	if strings.TrimSpace(selection.Template) == "" {
		if strings.TrimSpace(selection.ToolID) == "" {
			a.logAIInteraction(AIInteractionLogEntry{
				Provider:          settings.Provider,
				Model:             settings.Model,
				BaseURL:           settings.BaseURL,
				Mode:              "run_template_tool",
				TerminalID:        id,
				TerminalType:      terminalType,
				TerminalName:      terminalName,
				Goal:              goal,
				Prompt:            prompt,
				TerminalOutput:    loggedTerminalOutput,
				ContextRedactions: loggedContextRedactions,
				ContextAudit:      contextAudit,
				Response:          result,
				Error:             "AI did not select a runnable tool",
				Variables:         selection.Variables,
				Reason:            selection.Reason,
			})
			return "", fmt.Errorf("AI did not select a runnable tool")
		}
	}

	selectedTool, err := resolveSelectedTool(registry, selection, config)
	if err != nil {
		a.logAIInteraction(AIInteractionLogEntry{
			Provider:          settings.Provider,
			Model:             settings.Model,
			BaseURL:           settings.BaseURL,
			Mode:              "run_template_tool",
			TerminalID:        id,
			TerminalType:      terminalType,
			TerminalName:      terminalName,
			Goal:              goal,
			Prompt:            prompt,
			TerminalOutput:    loggedTerminalOutput,
			ContextRedactions: loggedContextRedactions,
			ContextAudit:      contextAudit,
			Response:          result,
			Error:             err.Error(),
			Template:          firstNonEmpty(selectedToolName(selection), selection.Template),
			Variables:         selection.Variables,
			Reason:            selection.Reason,
		})
		return "", err
	}
	discoveryResolver := aiflow.NewCachedDiscoveryResolver(a.getTemplateContext().CurrentDir)
	if err := aiflow.ValidateSelectedToolWithDiscovery(selectedTool, selection.Variables, terminalType, discoveryResolver); err != nil {
		a.logAIInteraction(AIInteractionLogEntry{
			Provider:          settings.Provider,
			Model:             settings.Model,
			BaseURL:           settings.BaseURL,
			Mode:              "run_template_tool",
			TerminalID:        id,
			TerminalType:      terminalType,
			TerminalName:      terminalName,
			Goal:              goal,
			Prompt:            prompt,
			TerminalOutput:    loggedTerminalOutput,
			ContextRedactions: loggedContextRedactions,
			ContextAudit:      contextAudit,
			Response:          result,
			Error:             err.Error(),
			Template:          selectedTool.Name(),
			Variables:         selection.Variables,
			Reason:            selection.Reason,
		})
		return "", err
	}

	stepID := fmt.Sprintf("ai-step-%d", time.Now().UnixNano())
	definition := workflow.Definition{
		ID:          fmt.Sprintf("%s-%d", config.Execution.WorkflowIDPrefix, time.Now().UnixNano()),
		Name:        config.Execution.WorkflowName,
		Description: strings.TrimSpace(goal),
		Mode:        config.Execution.WorkflowMode,
		Steps: []workflow.Step{
			{
				ID:               stepID,
				Type:             workflow.StepRunTool,
				Tool:             selectedTool.ID(),
				Inputs:           sortedStringMap(selection.Variables),
				RequiresApproval: config.Execution.ForceRequiresApproval,
			},
		},
	}

	engine := workflow.NewEngine(
		workflow.NewRunToolExecutor(registry, aiflow.NewApprovalPolicy(config.Approval)),
	)
	runCtx := workflow.RunContext{
		ToolContext: tools.RunContext{
			TerminalID:   id,
			TerminalType: terminalType,
			Writer:       p,
			TemplateData: a.getTemplateContext(),
		},
	}

	state, err := engine.Run(runCtx, definition)
	if err != nil {
		a.logAIInteraction(AIInteractionLogEntry{
			Provider:          settings.Provider,
			Model:             settings.Model,
			BaseURL:           settings.BaseURL,
			Mode:              "run_template_tool",
			TerminalID:        id,
			TerminalType:      terminalType,
			TerminalName:      terminalName,
			Goal:              goal,
			Prompt:            prompt,
			TerminalOutput:    loggedTerminalOutput,
			ContextRedactions: loggedContextRedactions,
			ContextAudit:      contextAudit,
			Response:          result,
			Error:             err.Error(),
			Template:          selectedTool.Name(),
			Variables:         selection.Variables,
			Reason:            selection.Reason,
		})
		return "", err
	}

	if state != nil && state.PendingApproval != nil {
		message := fmt.Sprintf(
			"Approval required before running tool: %s\nRisk: %s\nReason: %s",
			state.PendingApproval.ToolName,
			state.PendingApproval.Risk,
			state.PendingApproval.Reason,
		)
		a.logAIInteraction(AIInteractionLogEntry{
			Provider:          settings.Provider,
			Model:             settings.Model,
			BaseURL:           settings.BaseURL,
			Mode:              "run_template_tool",
			TerminalID:        id,
			TerminalType:      terminalType,
			TerminalName:      terminalName,
			Goal:              goal,
			Prompt:            prompt,
			TerminalOutput:    loggedTerminalOutput,
			ContextRedactions: loggedContextRedactions,
			ContextAudit:      contextAudit,
			Response:          result,
			Template:          selectedTool.Name(),
			Variables:         selection.Variables,
			Reason:            selection.Reason,
			Error:             "approval required",
		})
		return message, nil
	}

	entry := AIInteractionLogEntry{
		Provider:          settings.Provider,
		Model:             settings.Model,
		BaseURL:           settings.BaseURL,
		Mode:              "run_template_tool",
		TerminalID:        id,
		TerminalType:      terminalType,
		TerminalName:      terminalName,
		Goal:              goal,
		Prompt:            prompt,
		TerminalOutput:    loggedTerminalOutput,
		ContextRedactions: loggedContextRedactions,
		ContextAudit:      contextAudit,
		Response:          result,
		Template:          selectedTool.Name(),
		Variables:         selection.Variables,
		Reason:            selection.Reason,
	}
	a.logAIInteraction(entry)

	if strings.TrimSpace(selection.Reason) == "" {
		return fmt.Sprintf("Executed tool: %s", selectedTool.Name()), nil
	}

	return fmt.Sprintf("Executed tool: %s\nReason: %s", selectedTool.Name(), selection.Reason), nil
}

func selectedToolName(selection toolSelection) string {
	if strings.TrimSpace(selection.ToolID) != "" {
		return selection.ToolID
	}
	return strings.TrimSpace(selection.Template)
}
