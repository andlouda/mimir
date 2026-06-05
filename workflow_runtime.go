package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"mimir/aiflow"
	"mimir/tools"
	"mimir/workflow"
)

type workflowAIRunner struct {
	app *App
}

func (r workflowAIRunner) Run(prompt string, terminalType string, terminalName string, terminalOutput string) (string, error) {
	if r.app == nil {
		return "", fmt.Errorf("app is required")
	}

	if strings.HasPrefix(prompt, "AI mode: ") {
		mode := strings.TrimPrefix(prompt, "AI mode: ")
		builtIn, err := buildAIPrompt(mode, "", terminalType, terminalName, terminalOutput)
		if err == nil {
			prompt = builtIn
		}
	}

	settings := r.app.currentAISettings()
	sanitized := sanitizeTerminalOutputForAI(settings.Provider, terminalOutput)
	input := fmt.Sprintf(
		"You are a cautious infrastructure troubleshooting assistant. Use the supplied terminal context and answer the workflow step instruction clearly and practically. Prefer read-only reasoning, highlight uncertainty, and do not use markdown code fences.\n\nTerminal type: %s\nTerminal name: %s\nRecent terminal output:\n%s\n\nWorkflow step instruction:\n%s\n",
		terminalType,
		terminalName,
		sanitized.Value,
		strings.TrimSpace(prompt),
	)

	result, err := r.app.callAIProvider(settings, input)
	entry := AIInteractionLogEntry{
		Provider:          settings.Provider,
		Model:             settings.Model,
		BaseURL:           settings.BaseURL,
		Mode:              "workflow_step_ai",
		TerminalType:      terminalType,
		TerminalName:      terminalName,
		Goal:              strings.TrimSpace(prompt),
		Prompt:            input,
		TerminalOutput:    sanitized.Value,
		ContextRedactions: sanitized.Redactions,
		Response:          result,
	}
	if err != nil {
		entry.Error = err.Error()
	}
	r.app.logAIInteraction(entry)
	return result, err
}

func (a *App) RunWorkflowDraftJSON(definitionJSON string, terminalID int, terminalType string, terminalName string, terminalOutput string) (string, error) {
	definition, engine, runCtx, err := a.prepareWorkflowExecution(definitionJSON, terminalID, terminalType, terminalName, terminalOutput)
	if err != nil {
		return "", err
	}
	state, err := engine.Run(runCtx, definition)
	if err != nil {
		return "", err
	}

	return encodeWorkflowState(state)
}

func (a *App) ResumeWorkflowDraftJSON(definitionJSON string, stateJSON string, terminalID int, terminalType string, terminalName string, terminalOutput string) (string, error) {
	definition, engine, runCtx, err := a.prepareWorkflowExecution(definitionJSON, terminalID, terminalType, terminalName, terminalOutput)
	if err != nil {
		return "", err
	}

	var state workflow.State
	if err := strictUnmarshalJSON(stateJSON, &state); err != nil {
		return "", fmt.Errorf("failed to parse workflow state: %w", err)
	}
	if err := workflow.ValidateStateForDefinition(definition, &state); err != nil {
		return "", err
	}

	resumed, err := engine.ResumeApproved(runCtx, definition, &state)
	if err != nil {
		return "", err
	}
	return encodeWorkflowState(resumed)
}

func (a *App) RejectWorkflowDraftJSON(stateJSON string, reason string) (string, error) {
	var state workflow.State
	if err := strictUnmarshalJSON(stateJSON, &state); err != nil {
		return "", fmt.Errorf("failed to parse workflow state: %w", err)
	}
	if strings.TrimSpace(state.WorkflowID) == "" {
		return "", fmt.Errorf("workflow state requires a workflow ID")
	}

	if err := workflow.RejectPendingApproval(&state, strings.TrimSpace(reason)); err != nil {
		return "", err
	}
	return encodeWorkflowState(&state)
}

func (a *App) prepareWorkflowExecution(definitionJSON string, terminalID int, terminalType string, terminalName string, terminalOutput string) (workflow.Definition, *workflow.Engine, workflow.RunContext, error) {
	var definition workflow.Definition
	if err := strictUnmarshalJSON(definitionJSON, &definition); err != nil {
		return workflow.Definition{}, nil, workflow.RunContext{}, fmt.Errorf("failed to parse workflow draft: %w", err)
	}
	if err := workflow.ValidateDefinition(definition); err != nil {
		return workflow.Definition{}, nil, workflow.RunContext{}, err
	}

	registry, availableTools, err := buildAIToolRegistry(a.TemplateManager)
	if err != nil {
		return workflow.Definition{}, nil, workflow.RunContext{}, err
	}
	registry = tools.NewRegistry()
	for _, tool := range availableTools {
		if err := registry.Register(tool); err != nil {
			return workflow.Definition{}, nil, workflow.RunContext{}, err
		}
	}

	var writer templateWriter
	if requiresWorkflowWriter(definition) {
		pty, ok := a.TerminalManager.GetPty(terminalID)
		if !ok {
			return workflow.Definition{}, nil, workflow.RunContext{}, fmt.Errorf("workflow contains tool steps but terminal %d is not available", terminalID)
		}
		writer = pty
	}

	runCtx := workflow.RunContext{
		ToolContext: tools.RunContext{
			TerminalID:   terminalID,
			TerminalType: terminalType,
			Writer:       writer,
			TemplateData: a.getTemplateContext(),
		},
		DiscoveryResolver: aiflow.NewCachedDiscoveryResolver(a.getTemplateContext().CurrentDir),
		TerminalName:      terminalName,
		TerminalOutput:    terminalOutput,
		AIRunner:          workflowAIRunner{app: a},
	}

	engine := workflow.NewEngine(
		workflow.NewRunToolExecutor(registry, nil),
		workflow.NewRunDiscoveryExecutor(),
		workflow.NewAskAIExecutor(),
	)

	return definition, engine, runCtx, nil
}

func encodeWorkflowState(state *workflow.State) (string, error) {
	payload, err := json.Marshal(state)
	if err != nil {
		return "", fmt.Errorf("failed to encode workflow state: %w", err)
	}
	return string(payload), nil
}

type templateWriter interface {
	Write([]byte) (int, error)
}

func requiresWorkflowWriter(definition workflow.Definition) bool {
	for _, step := range definition.Steps {
		if step.Type == workflow.StepRunTool {
			return true
		}
	}
	return false
}
