package workflow

import (
	"fmt"
	"time"

	"mimir/activitylog"
	"mimir/tools"
)

// RunContext contains execution dependencies for a workflow run.
type RunContext struct {
	ToolContext       tools.RunContext
	DiscoveryResolver DiscoveryResolver
	TerminalName      string
	TerminalOutput    string
	AIRunner          AIRunner
}

// StepExecutor handles one or more workflow step types.
type StepExecutor interface {
	CanHandle(step Step) bool
	Execute(runCtx RunContext, state *State, step Step) error
}

// Engine executes workflow steps through registered executors.
type Engine struct {
	executors []StepExecutor
}

// NewEngine creates a workflow engine with a fixed executor set.
func NewEngine(executors ...StepExecutor) *Engine {
	return &Engine{
		executors: append([]StepExecutor(nil), executors...),
	}
}

// Run executes all steps of a workflow definition in order.
func (e *Engine) Run(runCtx RunContext, definition Definition) (*State, error) {
	state := &State{
		WorkflowID:    definition.ID,
		StepIndex:     0,
		Inputs:        map[string]string{},
		Outputs:       map[string]string{},
		Discovery:     map[string][]string{},
		Events:        []Event{},
		ApprovedSteps: map[string]bool{},
	}
	_ = activitylog.Append(activitylog.KindWorkflowRuns, activitylog.WorkflowRunEntry{
		Timestamp:  time.Now().Format(time.RFC3339),
		WorkflowID: definition.ID,
		Event:      "started",
		Message:    "Workflow started",
	})

	return e.runFromIndex(runCtx, definition, state, 0)
}

// ResumeApproved continues a paused workflow after the pending step was approved.
func (e *Engine) ResumeApproved(runCtx RunContext, definition Definition, state *State) (*State, error) {
	if state == nil {
		return nil, fmt.Errorf("workflow state is required")
	}
	if state.PendingApproval == nil {
		return nil, fmt.Errorf("workflow is not waiting for approval")
	}

	if state.ApprovedSteps == nil {
		state.ApprovedSteps = map[string]bool{}
	}
	state.ApprovedSteps[state.PendingApproval.StepID] = true
	state.Events = append(state.Events, Event{
		StepID:  state.PendingApproval.StepID,
		Type:    "approval_granted",
		Message: state.PendingApproval.Reason,
		Metadata: map[string]string{
			"tool_id":   state.PendingApproval.ToolID,
			"tool_name": state.PendingApproval.ToolName,
			"risk":      string(state.PendingApproval.Risk),
		},
	})
	_ = activitylog.Append(activitylog.KindApprovalEvents, activitylog.ApprovalEventEntry{
		Timestamp:  time.Now().Format(time.RFC3339),
		WorkflowID: state.WorkflowID,
		StepID:     state.PendingApproval.StepID,
		ToolID:     state.PendingApproval.ToolID,
		ToolName:   state.PendingApproval.ToolName,
		Risk:       string(state.PendingApproval.Risk),
		Event:      "granted",
		Reason:     state.PendingApproval.Reason,
	})
	state.PendingApproval = nil

	return e.runFromIndex(runCtx, definition, state, state.StepIndex)
}

func (e *Engine) runFromIndex(runCtx RunContext, definition Definition, state *State, startIndex int) (*State, error) {
	for index, step := range definition.Steps {
		if index < startIndex {
			continue
		}
		state.StepIndex = index
		if err := e.RunStep(runCtx, state, step); err != nil {
			if _, ok := err.(*ApprovalRequiredError); ok {
				state.Events = append(state.Events, Event{
					StepID:  step.ID,
					Type:    "workflow_paused",
					Message: err.Error(),
				})
				_ = activitylog.Append(activitylog.KindWorkflowRuns, activitylog.WorkflowRunEntry{
					Timestamp:  time.Now().Format(time.RFC3339),
					WorkflowID: state.WorkflowID,
					Event:      "paused",
					StepID:     step.ID,
					Message:    err.Error(),
				})
				return state, nil
			}
			state.Events = append(state.Events, Event{
				StepID:  step.ID,
				Type:    "step_failed",
				Message: err.Error(),
			})
			_ = activitylog.Append(activitylog.KindWorkflowRuns, activitylog.WorkflowRunEntry{
				Timestamp:  time.Now().Format(time.RFC3339),
				WorkflowID: state.WorkflowID,
				Event:      "failed",
				StepID:     step.ID,
				Message:    err.Error(),
			})
			return state, err
		}
	}

	state.StepIndex = len(definition.Steps)
	state.Events = append(state.Events, Event{
		Type:    "workflow_completed",
		Message: "Workflow completed",
	})
	_ = activitylog.Append(activitylog.KindWorkflowRuns, activitylog.WorkflowRunEntry{
		Timestamp:  time.Now().Format(time.RFC3339),
		WorkflowID: state.WorkflowID,
		Event:      "completed",
		Message:    "Workflow completed",
	})

	return state, nil
}

// RunStep executes a single step using the first matching executor.
func (e *Engine) RunStep(runCtx RunContext, state *State, step Step) error {
	for _, executor := range e.executors {
		if executor.CanHandle(step) {
			return executor.Execute(runCtx, state, step)
		}
	}

	return fmt.Errorf("no executor registered for step type %s", step.Type)
}

// RunToolExecutor executes `run_tool` steps through the tool registry.
type RunToolExecutor struct {
	registry *tools.Registry
	policy   ApprovalPolicy
}

// NewRunToolExecutor creates a step executor for tool-backed workflow steps.
func NewRunToolExecutor(registry *tools.Registry, policy ApprovalPolicy) *RunToolExecutor {
	if policy == nil {
		policy = NewDefaultApprovalPolicy()
	}
	return &RunToolExecutor{
		registry: registry,
		policy:   policy,
	}
}

func (e *RunToolExecutor) CanHandle(step Step) bool {
	return step.Type == StepRunTool
}

func (e *RunToolExecutor) Execute(runCtx RunContext, state *State, step Step) error {
	if e.registry == nil {
		return fmt.Errorf("tool registry is required")
	}
	if step.Tool == "" {
		return fmt.Errorf("workflow step %s has no tool configured", step.ID)
	}

	tool, ok := e.registry.Get(step.Tool)
	if !ok {
		return fmt.Errorf("tool %s not found", step.Tool)
	}

	if state.ApprovedSteps == nil {
		state.ApprovedSteps = map[string]bool{}
	}
	if !state.ApprovedSteps[step.ID] {
		decision := e.policy.Decide(ModeManual, step, tool)
		if decision.Required {
			state.PendingApproval = &PendingApproval{
				StepID:   step.ID,
				ToolID:   tool.ID(),
				ToolName: tool.Name(),
				Risk:     decision.Risk,
				Reason:   decision.Reason,
			}
			state.Events = append(state.Events, Event{
				StepID:  step.ID,
				Type:    "step_pending_approval",
				Message: decision.Reason,
				Metadata: map[string]string{
					"tool_id":   tool.ID(),
					"tool_name": tool.Name(),
					"risk":      string(decision.Risk),
				},
			})
			_ = activitylog.Append(activitylog.KindApprovalEvents, activitylog.ApprovalEventEntry{
				Timestamp:  time.Now().Format(time.RFC3339),
				WorkflowID: state.WorkflowID,
				StepID:     step.ID,
				ToolID:     tool.ID(),
				ToolName:   tool.Name(),
				Risk:       string(decision.Risk),
				Event:      "requested",
				Reason:     decision.Reason,
			})
			return &ApprovalRequiredError{
				StepID: step.ID,
				Reason: decision.Reason,
			}
		}
	}
	delete(state.ApprovedSteps, step.ID)

	inputs := map[string]string{}
	for key, value := range step.Inputs {
		inputs[key] = value
	}

	state.Events = append(state.Events, Event{
		StepID:  step.ID,
		Type:    "step_started",
		Message: fmt.Sprintf("Running tool %s", tool.Name()),
		Metadata: map[string]string{
			"tool_id":   tool.ID(),
			"tool_name": tool.Name(),
		},
	})
	_ = activitylog.Append(activitylog.KindToolExecutions, activitylog.ToolExecutionEntry{
		Timestamp:    time.Now().Format(time.RFC3339),
		Source:       "workflow",
		WorkflowID:   state.WorkflowID,
		StepID:       step.ID,
		ToolID:       tool.ID(),
		ToolName:     tool.Name(),
		TerminalID:   runCtx.ToolContext.TerminalID,
		TerminalType: runCtx.ToolContext.TerminalType,
		Inputs:       inputs,
	})

	result, err := tool.Run(runCtx.ToolContext, inputs)
	if err != nil {
		_ = activitylog.Append(activitylog.KindToolExecutions, activitylog.ToolExecutionEntry{
			Timestamp:    time.Now().Format(time.RFC3339),
			Source:       "workflow",
			WorkflowID:   state.WorkflowID,
			StepID:       step.ID,
			ToolID:       tool.ID(),
			ToolName:     tool.Name(),
			TerminalID:   runCtx.ToolContext.TerminalID,
			TerminalType: runCtx.ToolContext.TerminalType,
			Inputs:       inputs,
			Error:        err.Error(),
		})
		return err
	}

	if state.Outputs == nil {
		state.Outputs = map[string]string{}
	}
	if result.Output != "" {
		state.Outputs[step.ID] = result.Output
	}

	metadata := map[string]string{
		"tool_id":   tool.ID(),
		"tool_name": tool.Name(),
	}
	for key, value := range result.Metadata {
		metadata[key] = value
	}

	state.Events = append(state.Events, Event{
		StepID:   step.ID,
		Type:     "step_completed",
		Message:  result.Output,
		Metadata: metadata,
	})
	_ = activitylog.Append(activitylog.KindToolExecutions, activitylog.ToolExecutionEntry{
		Timestamp:    time.Now().Format(time.RFC3339),
		Source:       "workflow",
		WorkflowID:   state.WorkflowID,
		StepID:       step.ID,
		ToolID:       tool.ID(),
		ToolName:     tool.Name(),
		TerminalID:   runCtx.ToolContext.TerminalID,
		TerminalType: runCtx.ToolContext.TerminalType,
		Inputs:       inputs,
		Output:       result.Output,
		Metadata:     result.Metadata,
	})

	return nil
}

// RunDiscoveryExecutor executes `run_discovery` steps through a discovery resolver.
type RunDiscoveryExecutor struct{}

// NewRunDiscoveryExecutor creates a step executor for discovery-backed workflow steps.
func NewRunDiscoveryExecutor() *RunDiscoveryExecutor {
	return &RunDiscoveryExecutor{}
}

func (e *RunDiscoveryExecutor) CanHandle(step Step) bool {
	return step.Type == StepRunDiscovery
}

func (e *RunDiscoveryExecutor) Execute(runCtx RunContext, state *State, step Step) error {
	if runCtx.DiscoveryResolver == nil {
		return fmt.Errorf("discovery resolver is required")
	}
	if step.DiscoveryTool == "" {
		return fmt.Errorf("workflow step %s has no discovery tool configured", step.ID)
	}

	inputs := map[string]string{}
	for key, value := range step.Inputs {
		inputs[key] = value
	}

	state.Events = append(state.Events, Event{
		StepID:  step.ID,
		Type:    "step_started",
		Message: fmt.Sprintf("Running discovery %s", step.DiscoveryTool),
		Metadata: map[string]string{
			"discovery_tool": step.DiscoveryTool,
		},
	})

	values, err := runCtx.DiscoveryResolver.Resolve(step.DiscoveryTool, runCtx.ToolContext.TerminalType, inputs)
	if err != nil {
		return err
	}

	if state.Outputs == nil {
		state.Outputs = map[string]string{}
	}
	if state.Discovery == nil {
		state.Discovery = map[string][]string{}
	}
	state.Discovery[step.ID] = append([]string(nil), values...)
	state.Outputs[step.ID] = fmt.Sprintf("%d discovery values", len(values))

	metadata := map[string]string{
		"discovery_tool": step.DiscoveryTool,
		"count":          fmt.Sprintf("%d", len(values)),
	}
	state.Events = append(state.Events, Event{
		StepID:   step.ID,
		Type:     "step_completed",
		Message:  state.Outputs[step.ID],
		Metadata: metadata,
	})
	_ = activitylog.Append(activitylog.KindToolExecutions, activitylog.ToolExecutionEntry{
		Timestamp:    time.Now().Format(time.RFC3339),
		Source:       "workflow",
		WorkflowID:   state.WorkflowID,
		StepID:       step.ID,
		ToolID:       step.DiscoveryTool,
		ToolName:     step.DiscoveryTool,
		TerminalID:   runCtx.ToolContext.TerminalID,
		TerminalType: runCtx.ToolContext.TerminalType,
		Inputs:       inputs,
		Output:       state.Outputs[step.ID],
		Metadata:     metadata,
	})

	return nil
}

// AskAIExecutor executes `ask_ai` steps through a workflow AI runner.
type AskAIExecutor struct{}

// NewAskAIExecutor creates a step executor for AI-backed workflow steps.
func NewAskAIExecutor() *AskAIExecutor {
	return &AskAIExecutor{}
}

func (e *AskAIExecutor) CanHandle(step Step) bool {
	return step.Type == StepAskAI
}

func (e *AskAIExecutor) Execute(runCtx RunContext, state *State, step Step) error {
	if runCtx.AIRunner == nil {
		return fmt.Errorf("AI runner is required")
	}
	if step.Prompt == "" && step.AIMode == "" {
		return fmt.Errorf("workflow step %s has no AI prompt configured", step.ID)
	}

	state.Events = append(state.Events, Event{
		StepID:  step.ID,
		Type:    "step_started",
		Message: "Running AI step",
		Metadata: map[string]string{
			"kind": "ask_ai",
		},
	})

	prompt := step.Prompt
	if prompt == "" && step.AIMode != "" {
		prompt = "AI mode: " + step.AIMode
	}
	result, err := runCtx.AIRunner.Run(prompt, runCtx.ToolContext.TerminalType, runCtx.TerminalName, runCtx.TerminalOutput)
	if err != nil {
		return err
	}

	if state.Outputs == nil {
		state.Outputs = map[string]string{}
	}
	state.Outputs[step.ID] = result
	state.Events = append(state.Events, Event{
		StepID:  step.ID,
		Type:    "step_completed",
		Message: "AI step completed",
		Metadata: map[string]string{
			"kind": "ask_ai",
		},
	})
	return nil
}
