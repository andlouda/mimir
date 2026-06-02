package workflow

import (
	"fmt"
	"testing"

	"mimir/tools"
)

type stubDiscoveryResolver struct {
	lastTool  string
	lastInput map[string]string
	values    []string
	err       error
}

type stubAIRunner struct {
	lastPrompt string
	result     string
	err        error
}

func (s *stubAIRunner) Run(prompt string, _ string, _ string, _ string) (string, error) {
	s.lastPrompt = prompt
	if s.err != nil {
		return "", s.err
	}
	return s.result, nil
}

func (s *stubDiscoveryResolver) Resolve(discoveryTool string, _ string, variables map[string]string) ([]string, error) {
	s.lastTool = discoveryTool
	s.lastInput = variables
	if s.err != nil {
		return nil, s.err
	}
	return append([]string(nil), s.values...), nil
}

type stubTool struct {
	id          string
	name        string
	description string
	category    string
	risk        tools.RiskLevel
	class       tools.ToolClass
	output      string
	err         error
	lastInput   map[string]string
}

func (s *stubTool) ID() string                    { return s.id }
func (s *stubTool) Name() string                  { return s.name }
func (s *stubTool) Description() string           { return s.description }
func (s *stubTool) Category() string              { return s.category }
func (s *stubTool) Risk() tools.RiskLevel         { return s.risk }
func (s *stubTool) Class() tools.ToolClass        { return s.class }
func (s *stubTool) Parameters() []tools.Parameter { return nil }
func (s *stubTool) Run(_ tools.RunContext, input map[string]string) (tools.ToolResult, error) {
	s.lastInput = input
	if s.err != nil {
		return tools.ToolResult{}, s.err
	}
	return tools.ToolResult{
		Output: s.output,
		Metadata: map[string]string{
			"ran": "true",
		},
	}, nil
}

func TestEngineRunToolWorkflow(t *testing.T) {
	registry := tools.NewRegistry()
	tool := &stubTool{
		id:     "template:list-files",
		name:   "List Files",
		output: "Executed list files",
	}
	if err := registry.Register(tool); err != nil {
		t.Fatalf("register tool: %v", err)
	}

	engine := NewEngine(NewRunToolExecutor(registry, nil))
	definition := Definition{
		ID:   "workflow:list-files",
		Name: "List Files Workflow",
		Mode: ModeManual,
		Steps: []Step{
			{
				ID:   "step-1",
				Type: StepRunTool,
				Tool: "template:list-files",
				Inputs: map[string]string{
					"path": ".",
				},
			},
		},
	}

	state, err := engine.Run(RunContext{}, definition)
	if err != nil {
		t.Fatalf("run workflow: %v", err)
	}

	if state.WorkflowID != "workflow:list-files" {
		t.Fatalf("unexpected workflow id: %s", state.WorkflowID)
	}
	if got := state.Outputs["step-1"]; got != "Executed list files" {
		t.Fatalf("unexpected step output: %q", got)
	}
	if tool.lastInput["path"] != "." {
		t.Fatalf("expected tool input to be forwarded, got %+v", tool.lastInput)
	}
	if len(state.Events) != 3 {
		t.Fatalf("expected start, complete, workflow-complete events; got %+v", state.Events)
	}
	if state.Events[0].Type != "step_started" || state.Events[1].Type != "step_completed" || state.Events[2].Type != "workflow_completed" {
		t.Fatalf("unexpected event order: %+v", state.Events)
	}
}

func TestEngineRunFailsWithoutExecutor(t *testing.T) {
	engine := NewEngine()
	_, err := engine.Run(RunContext{}, Definition{
		ID: "workflow:no-executor",
		Steps: []Step{
			{ID: "step-1", Type: StepRunTool, Tool: "missing"},
		},
	})
	if err == nil {
		t.Fatal("expected workflow to fail without executor")
	}
}

func TestEngineRunCapturesToolFailure(t *testing.T) {
	registry := tools.NewRegistry()
	tool := &stubTool{
		id:   "template:failing",
		name: "Failing Tool",
		err:  fmt.Errorf("boom"),
	}
	if err := registry.Register(tool); err != nil {
		t.Fatalf("register tool: %v", err)
	}

	engine := NewEngine(NewRunToolExecutor(registry, nil))
	state, err := engine.Run(RunContext{}, Definition{
		ID: "workflow:failing",
		Steps: []Step{
			{ID: "step-1", Type: StepRunTool, Tool: "template:failing"},
		},
	})
	if err == nil {
		t.Fatal("expected workflow run to fail")
	}
	if len(state.Events) < 2 {
		t.Fatalf("expected failure events, got %+v", state.Events)
	}
	if state.Events[len(state.Events)-1].Type != "step_failed" {
		t.Fatalf("expected trailing step_failed event, got %+v", state.Events)
	}
}

func TestEnginePausesForApproval(t *testing.T) {
	registry := tools.NewRegistry()
	tool := &stubTool{
		id:     "template:dangerous",
		name:   "Dangerous Tool",
		risk:   tools.RiskMedium,
		output: "Executed dangerous tool",
	}
	if err := registry.Register(tool); err != nil {
		t.Fatalf("register tool: %v", err)
	}

	engine := NewEngine(NewRunToolExecutor(registry, nil))
	state, err := engine.Run(RunContext{}, Definition{
		ID: "workflow:approval",
		Steps: []Step{
			{ID: "step-1", Type: StepRunTool, Tool: "template:dangerous"},
		},
	})
	if err != nil {
		t.Fatalf("expected paused workflow without hard error, got %v", err)
	}
	if state.PendingApproval == nil {
		t.Fatal("expected workflow to be pending approval")
	}
	if tool.lastInput != nil {
		t.Fatalf("tool should not have run before approval, got input %+v", tool.lastInput)
	}
	if state.Events[len(state.Events)-1].Type != "workflow_paused" {
		t.Fatalf("expected workflow_paused event, got %+v", state.Events)
	}
}

func TestEngineResumeApprovedStep(t *testing.T) {
	registry := tools.NewRegistry()
	tool := &stubTool{
		id:     "template:dangerous",
		name:   "Dangerous Tool",
		risk:   tools.RiskHigh,
		output: "Executed after approval",
	}
	if err := registry.Register(tool); err != nil {
		t.Fatalf("register tool: %v", err)
	}

	engine := NewEngine(NewRunToolExecutor(registry, nil))
	definition := Definition{
		ID: "workflow:resume",
		Steps: []Step{
			{ID: "step-1", Type: StepRunTool, Tool: "template:dangerous"},
			{ID: "step-2", Type: StepRunTool, Tool: "template:dangerous"},
		},
	}

	state, err := engine.Run(RunContext{}, definition)
	if err != nil {
		t.Fatalf("initial run returned unexpected error: %v", err)
	}
	if state.PendingApproval == nil {
		t.Fatal("expected pending approval after first run")
	}

	resumed, err := engine.ResumeApproved(RunContext{}, definition, state)
	if err != nil {
		t.Fatalf("resume approved returned error: %v", err)
	}
	if resumed.PendingApproval == nil {
		t.Fatal("expected second high-risk step to request approval again")
	}
	if got := resumed.Outputs["step-1"]; got != "Executed after approval" {
		t.Fatalf("expected first step output after resume, got %q", got)
	}
}

func TestEngineRunDiscoveryWorkflow(t *testing.T) {
	resolver := &stubDiscoveryResolver{
		values: []string{"default", "kube-system"},
	}

	engine := NewEngine(NewRunDiscoveryExecutor())
	state, err := engine.Run(RunContext{
		DiscoveryResolver: resolver,
	}, Definition{
		ID:   "workflow:discovery",
		Name: "Discovery Workflow",
		Steps: []Step{
			{
				ID:            "step-1",
				Type:          StepRunDiscovery,
				DiscoveryTool: "discovery:list_k8s_namespaces",
			},
		},
	})
	if err != nil {
		t.Fatalf("run discovery workflow: %v", err)
	}
	if resolver.lastTool != "discovery:list_k8s_namespaces" {
		t.Fatalf("unexpected discovery tool: %s", resolver.lastTool)
	}
	if got := state.Outputs["step-1"]; got != "2 discovery values" {
		t.Fatalf("unexpected discovery output summary: %q", got)
	}
	if len(state.Discovery["step-1"]) != 2 {
		t.Fatalf("expected discovery values to be stored, got %+v", state.Discovery)
	}
}

func TestEngineRunAskAIWorkflow(t *testing.T) {
	aiRunner := &stubAIRunner{result: "Likely root cause: service port mismatch."}
	engine := NewEngine(NewAskAIExecutor())
	state, err := engine.Run(RunContext{
		AIRunner: aiRunner,
	}, Definition{
		ID:   "workflow:ai",
		Name: "AI Workflow",
		Steps: []Step{
			{
				ID:     "step-1",
				Type:   StepAskAI,
				Prompt: "Explain the failure.",
			},
		},
	})
	if err != nil {
		t.Fatalf("run AI workflow: %v", err)
	}
	if aiRunner.lastPrompt != "Explain the failure." {
		t.Fatalf("unexpected AI prompt: %q", aiRunner.lastPrompt)
	}
	if got := state.Outputs["step-1"]; got != "Likely root cause: service port mismatch." {
		t.Fatalf("unexpected AI output: %q", got)
	}
}

func TestRejectPendingApproval(t *testing.T) {
	state := &State{
		WorkflowID: "workflow:deny",
		PendingApproval: &PendingApproval{
			StepID:   "step-1",
			ToolID:   "template:dangerous",
			ToolName: "Dangerous Tool",
			Risk:     tools.RiskHigh,
			Reason:   "high-risk tools require approval",
		},
	}

	if err := RejectPendingApproval(state, "approval denied by user"); err != nil {
		t.Fatalf("RejectPendingApproval returned error: %v", err)
	}
	if state.PendingApproval != nil {
		t.Fatal("expected pending approval to be cleared")
	}
	if len(state.Events) == 0 || state.Events[len(state.Events)-1].Type != "approval_denied" {
		t.Fatalf("expected approval_denied event, got %+v", state.Events)
	}
}
