package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"mimir/agents"
	"mimir/agentworkspace"
)

func workspaceStore() (*agentworkspace.Store, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	return agentworkspace.NewStore(filepath.Join(dir, "mimir", "agent_workspace.json")), nil
}

func workspaceJSON(data agentworkspace.Data, err error) (string, error) {
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(data)
	return string(b), err
}

func (a *App) GetAgentWorkspaceJSON() (string, error) {
	s, err := workspaceStore()
	if err != nil {
		return "", err
	}
	return workspaceJSON(s.Snapshot())
}

func (a *App) SaveAgentWorkspaceProjectJSON(raw string) (string, error) {
	var p agentworkspace.Project
	if len(raw) > 65536 || json.Unmarshal([]byte(raw), &p) != nil {
		return "", fmt.Errorf("invalid project data")
	}
	s, err := workspaceStore()
	if err != nil {
		return "", err
	}
	d, err := s.SaveProject(p)
	if err == nil {
		if p.ID == "" {
			p.ID = d.Projects[len(d.Projects)-1].ID
		}
		logAgentEvent("agent_workspace_project_saved", fmt.Sprintf("project %s; archived=%t", p.ID, p.Archived))
	}
	return workspaceJSON(d, err)
}

func (a *App) SaveAgentWorkspaceTaskJSON(raw string) (string, error) {
	var t agentworkspace.Task
	if len(raw) > 65536 || json.Unmarshal([]byte(raw), &t) != nil {
		return "", fmt.Errorf("invalid task data")
	}
	s, err := workspaceStore()
	if err != nil {
		return "", err
	}
	d, err := s.SaveTask(t)
	if err == nil {
		if t.ID == "" {
			t.ID = d.Tasks[len(d.Tasks)-1].ID
		}
		logAgentEvent("agent_workspace_task_saved", fmt.Sprintf("task %s; project %s; status=%s; archived=%t", t.ID, t.ProjectID, t.Status, t.Archived))
	}
	return workspaceJSON(d, err)
}

// Only references returned by discovery for this terminal can be linked. Host,
// kind and directory are resolved here, never trusted from frontend input.
func (a *App) LinkAgentWorkspaceSessionJSON(projectID, taskID string, terminalID int, terminalType, file string) (string, error) {
	if len(file) > 4096 {
		return "", fmt.Errorf("invalid session reference")
	}
	state, ok := a.rememberedAgent(terminalID)
	if !ok {
		return "", fmt.Errorf("no agent detected in this terminal")
	}
	raw, err := a.ListAgentSessionsJSON(terminalID, terminalType)
	if err != nil {
		return "", err
	}
	// Detection may have changed while reading remote session metadata.
	now, ok := a.rememberedAgent(terminalID)
	if !ok || now.kind != state.kind || now.pid != state.pid || now.cwd != state.cwd || now.source != state.source {
		return "", fmt.Errorf("agent changed; refresh the session list")
	}
	var list struct {
		Sessions []agents.SessionSummary `json:"sessions"`
	}
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return "", err
	}
	ref, err := discoveredWorkspaceSession(state, a.agentHost(terminalID, state.source), list.Sessions, file)
	if err != nil {
		return "", err
	}
	ref.ProjectID, ref.TaskID = projectID, taskID
	s, err := workspaceStore()
	if err != nil {
		return "", err
	}
	d, err := s.LinkSession(ref)
	if err == nil {
		for _, saved := range d.Sessions {
			if saved.Host == ref.Host && saved.Kind == ref.Kind && saved.File == ref.File {
				logAgentEvent("agent_workspace_session_linked", fmt.Sprintf("assignment %s; project %s; task %s", saved.ID, saved.ProjectID, saved.TaskID))
				break
			}
		}
	}
	return workspaceJSON(d, err)
}

func discoveredWorkspaceSession(state agentTerminalState, host string, candidates []agents.SessionSummary, file string) (agentworkspace.Session, error) {
	for _, candidate := range candidates {
		if candidate.File != file {
			continue
		}
		cwd := candidate.Cwd
		if cwd == "" {
			cwd = state.cwd
		}
		return agentworkspace.Session{Host: host, Kind: string(state.kind), File: candidate.File, Title: candidate.Title, Cwd: cwd}, nil
	}
	return agentworkspace.Session{}, fmt.Errorf("session was not found in this terminal's discovered sessions")
}

func (a *App) UnlinkAgentWorkspaceSessionJSON(id string) (string, error) {
	s, err := workspaceStore()
	if err != nil {
		return "", err
	}
	d, err := s.UnlinkSession(id)
	if err == nil {
		logAgentEvent("agent_workspace_session_unlinked", fmt.Sprintf("assignment %s removed", id))
	}
	return workspaceJSON(d, err)
}
