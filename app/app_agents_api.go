package main

// Agent bindings: the Wails-exported surface for the AI agent features.
// Everything is implemented in package agentsvc; these forwarders keep the
// frontend contract (window.go.main.App.*) unchanged.

func (a *App) AnswerAgentPermission(terminalID int, allow bool) error {
	return a.agents.AnswerAgentPermission(terminalID, allow)
}
func (a *App) DeleteClaudeSession(file string) error { return a.agents.DeleteClaudeSession(file) }
func (a *App) DetectAgentForTerminalJSON(terminalID int, terminalType string) (string, error) {
	return a.agents.DetectAgentForTerminalJSON(terminalID, terminalType)
}
func (a *App) GetAgentAnnotationsJSON() (string, error) { return a.agents.GetAgentAnnotationsJSON() }
func (a *App) GetAgentGitStatusJSON(terminalID int, terminalType string) (string, error) {
	return a.agents.GetAgentGitStatusJSON(terminalID, terminalType)
}
func (a *App) GetAgentPaneScreenJSON(terminalID int, terminalType string) (string, error) {
	return a.agents.GetAgentPaneScreenJSON(terminalID, terminalType)
}
func (a *App) GetAgentPaneTextJSON(terminalID int, terminalType string, full bool) (string, error) {
	return a.agents.GetAgentPaneTextJSON(terminalID, terminalType, full)
}
func (a *App) GetAgentProcessesJSON(terminalID int, terminalType string) (string, error) {
	return a.agents.GetAgentProcessesJSON(terminalID, terminalType)
}
func (a *App) GetAgentProjectJSON(terminalID int, terminalType string) (string, error) {
	return a.agents.GetAgentProjectJSON(terminalID, terminalType)
}
func (a *App) GetAgentTimelineJSON(terminalID int, afterSeq int, limit int) (string, error) {
	return a.agents.GetAgentTimelineJSON(terminalID, afterSeq, limit)
}
func (a *App) GetAgentTimelinePath(terminalID int) (string, error) {
	return a.agents.GetAgentTimelinePath(terminalID)
}
func (a *App) GetAgentTranscriptJSON(terminalID int, terminalType string, limit int) (string, error) {
	return a.agents.GetAgentTranscriptJSON(terminalID, terminalType, limit)
}
func (a *App) GetAgentWorkspaceJSON() (string, error) { return a.agents.GetAgentWorkspaceJSON() }
func (a *App) GetClaudeHookStatusForHostJSON(host string) string {
	return a.agents.GetClaudeHookStatusForHostJSON(host)
}
func (a *App) GetClaudeHookStatusJSON(terminalID int) string {
	return a.agents.GetClaudeHookStatusJSON(terminalID)
}
func (a *App) GetTmuxPasteBufferJSON(terminalID int, terminalType string) (string, error) {
	return a.agents.GetTmuxPasteBufferJSON(terminalID, terminalType)
}
func (a *App) InstallClaudeHook(terminalID int) error { return a.agents.InstallClaudeHook(terminalID) }
func (a *App) InstallClaudeHookOnHost(host string) error {
	return a.agents.InstallClaudeHookOnHost(host)
}
func (a *App) IsAgentDetectionEnabled() bool { return a.agents.IsAgentDetectionEnabled() }
func (a *App) LinkAgentWorkspaceSessionJSON(projectID, taskID string, terminalID int, terminalType, file string) (string, error) {
	return a.agents.LinkAgentWorkspaceSessionJSON(projectID, taskID, terminalID, terminalType, file)
}
func (a *App) ListAgentSessionsJSON(terminalID int, terminalType string) (string, error) {
	return a.agents.ListAgentSessionsJSON(terminalID, terminalType)
}
func (a *App) ListClaudeHookHostsJSON() string { return a.agents.ListClaudeHookHostsJSON() }
func (a *App) ListOfflineClaudeSessionsJSON(limit int) (string, error) {
	return a.agents.ListOfflineClaudeSessionsJSON(limit)
}
func (a *App) OpenAgentTimelineFolder() error           { return a.agents.OpenAgentTimelineFolder() }
func (a *App) RemoveClaudeHook(terminalID int) error    { return a.agents.RemoveClaudeHook(terminalID) }
func (a *App) RemoveClaudeHookOnHost(host string) error { return a.agents.RemoveClaudeHookOnHost(host) }
func (a *App) RerenderClaudeInPane(terminalID int, terminalType string) (string, error) {
	return a.agents.RerenderClaudeInPane(terminalID, terminalType)
}
func (a *App) SaveAgentWorkspaceProjectJSON(raw string) (string, error) {
	return a.agents.SaveAgentWorkspaceProjectJSON(raw)
}
func (a *App) SaveAgentWorkspaceTaskJSON(raw string) (string, error) {
	return a.agents.SaveAgentWorkspaceTaskJSON(raw)
}
func (a *App) SelectAgentSession(terminalID int, terminalType string, file string) error {
	return a.agents.SelectAgentSession(terminalID, terminalType, file)
}
func (a *App) SetAgentDetectionEnabled(enabled bool) error {
	return a.agents.SetAgentDetectionEnabled(enabled)
}
func (a *App) SetAgentProjectName(key, name string) error {
	return a.agents.SetAgentProjectName(key, name)
}
func (a *App) SetAgentSessionAnnotation(key, name, note string, archived bool) error {
	return a.agents.SetAgentSessionAnnotation(key, name, note, archived)
}
func (a *App) UnlinkAgentWorkspaceSessionJSON(id string) (string, error) {
	return a.agents.UnlinkAgentWorkspaceSessionJSON(id)
}
