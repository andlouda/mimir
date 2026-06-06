package main

import (
	"context"
	"embed"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"mimir/activitylog"
	"mimir/desktop"
	"mimir/folder"
	"mimir/history"
	"mimir/notes"
	"mimir/recording"
	"mimir/session"
	"mimir/ssh"
	"mimir/template"
	"mimir/terminal"
	"mimir/transcript"
	"mimir/update"
	"mimir/workflow"
)

// App struct
type App struct {
	ctx                  context.Context
	TerminalManager      *terminal.Manager
	TemplateManager      *template.Manager
	loadedSessionData    session.SessionData
	activeTerminalStates map[int]session.TerminalState
	stateMu              sync.Mutex
	aiSettings           AISettings
	aiMu                 sync.Mutex
	functionCatalog      []FunctionCatalogEntry
	functionCatalogJSON  string
	functionCatalogMu    sync.Mutex
	PlaybookStore        *workflow.PlaybookStore
	sshProfileStore      *ssh.ProfileStore
	sshSecretStore       *ssh.SecretStore
	knownHostStore       *ssh.KnownHostStore
	pendingHostKeys      map[string]pendingSSHHostKey
	pendingHostKeyMu     sync.Mutex
	historyStore         *history.Store
	noteStore            *notes.NoteStore
	recordingStore       *recording.Store
	folderStore          *folder.FolderStore
	appIconPNG           []byte
}

// NewApp creates a new App application struct
func NewApp(embeddedTemplates embed.FS, iconPNG []byte) *App {
	app := &App{
		TerminalManager:      terminal.NewManager(),
		TemplateManager:      template.NewManager(embeddedTemplates),
		activeTerminalStates: make(map[int]session.TerminalState),
		pendingHostKeys:      make(map[string]pendingSSHHostKey),
		appIconPNG:           iconPNG,
	}

	// Load templates during app initialization
	err := app.TemplateManager.LoadTemplates()
	if err != nil {
		log.Printf("Failed to load templates: %v", err)
	}

	// Load previous session data
	loadedData, err := session.LoadSession()
	if err != nil {
		log.Printf("Failed to load session: %v", err)
	} else {
		app.loadedSessionData = loadedData
	}

	// The secret store must exist before AI settings load, since the OpenAI
	// API key is retrieved through it.
	secretStore, err := ssh.NewSecretStore()
	if err != nil {
		log.Printf("Failed to initialize SSH secret store: %v", err)
	} else {
		app.sshSecretStore = secretStore
	}

	settings, err := LoadAISettings(app.sshSecretStore)
	if err != nil {
		log.Printf("Failed to load AI settings: %v", err)
	} else {
		app.aiSettings = settings
	}

	playbookStore, err := workflow.NewDefaultPlaybookStore()
	if err != nil {
		log.Printf("Failed to initialize playbook store: %v", err)
	} else {
		app.PlaybookStore = playbookStore
	}

	profileStore, err := ssh.NewProfileStore()
	if err != nil {
		log.Printf("Failed to initialize SSH profile store: %v", err)
	} else {
		app.sshProfileStore = profileStore
	}

	knownHosts, err := ssh.NewKnownHostStore()
	if err != nil {
		log.Printf("Failed to initialize known hosts store: %v", err)
	} else {
		app.knownHostStore = knownHosts
	}

	histStore, err := history.NewStore()
	if err != nil {
		log.Printf("Failed to initialize command history store: %v", err)
	} else {
		app.historyStore = histStore
		app.TerminalManager.SetHistoryStore(histStore)
	}

	noteStore, err := notes.NewNoteStore()
	if err != nil {
		log.Printf("Failed to initialize note store: %v", err)
	} else {
		app.noteStore = noteStore
	}

	recStore, err := recording.NewStore()
	if err != nil {
		log.Printf("Failed to initialize recording store: %v", err)
	} else {
		app.recordingStore = recStore
	}

	fldStore, err := folder.NewFolderStore()
	if err != nil {
		log.Printf("Failed to initialize folder store: %v", err)
	} else {
		app.folderStore = fldStore
	}

	return app
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.TerminalManager.SetContext(ctx)

	updateStarted, err := a.applyPendingUpdateOnStartup()
	if err != nil {
		log.Printf("Failed to apply pending update: %v", err)
	}
	if updateStarted {
		return
	}

	if err := desktop.Install(a.appIconPNG); err != nil {
		log.Printf("Desktop integration: %v", err)
	}
}

func (a *App) applyPendingUpdateOnStartup() (bool, error) {
	pending, err := update.ReadPendingMarker()
	if err != nil {
		return false, fmt.Errorf("check pending update: %w", err)
	}
	if pending == nil {
		return false, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return false, fmt.Errorf("resolve executable: %w", err)
	}
	if err := a.restartAfterPendingUpdate(pending, exe); err != nil {
		return false, err
	}
	return true, nil
}

// getTemplateContext gathers dynamic information for template execution.
func (a *App) getTemplateContext() template.TemplateContext {
	currentDir, _ := os.Getwd()
	currentUser, _ := user.Current()
	hostname, _ := os.Hostname()

	return template.TemplateContext{
		CurrentDir: currentDir,
		Username:   currentUser.Username,
		Hostname:   hostname,
		// SelectedText: "", // Not yet implemented
		// Clipboard:    "", // Not yet implemented
	}
}

// GetCurrentDirectory returns the current working directory.
func (a *App) GetCurrentDirectory() (string, error) {
	return os.Getwd()
}

// GetLoadedSessionData returns the session data loaded at startup.
// This is called by the frontend to restore previous terminals.
func (a *App) GetLoadedSessionData() session.SessionData {
	return a.loadedSessionData
}

// SaveCurrentSession collects and saves the current state of all active terminals.
func (a *App) SaveCurrentSession() error {
	a.stateMu.Lock()
	var terminalsToSave []session.TerminalState
	for _, state := range a.activeTerminalStates {
		terminalsToSave = append(terminalsToSave, state)
	}
	a.stateMu.Unlock()

	data := session.SessionData{
		Terminals: terminalsToSave,
	}

	return session.SaveSession(data)
}

// UpdateTerminalState is called by the frontend to update the state of a terminal.
func (a *App) UpdateTerminalState(id int, terminalType string, name string, minimized bool, sshProfileID string, tmuxSessionName string, resumeID string, restoreClass string, folderID string) {
	a.stateMu.Lock()
	defer a.stateMu.Unlock()
	transcriptPath := ""
	if resumeID != "" {
		if path, err := transcript.PathForResumeID(resumeID); err == nil {
			transcriptPath = path
		}
	}
	a.activeTerminalStates[id] = session.TerminalState{
		Type:            terminalType,
		Name:            name,
		Minimized:       minimized,
		SSHProfileID:    sshProfileID,
		TmuxSessionName: tmuxSessionName,
		ResumeID:        resumeID,
		TranscriptPath:  transcriptPath,
		RestoreClass:    restoreClass,
		FolderID:        folderID,
	}
}

func (a *App) AppendTerminalTranscript(resumeID string, data string) error {
	if resumeID == "" || data == "" {
		return nil
	}
	_, err := transcript.Append(resumeID, data)
	return err
}

func (a *App) GetTerminalTranscriptExcerpt(resumeID string, maxBytes int) (string, error) {
	if maxBytes <= 0 {
		maxBytes = 16000
	}
	return transcript.ReadTail(resumeID, maxBytes)
}

// TranscriptListEntry is what the frontend sees per stored transcript.
// Name / Type / SSHProfileID are best-effort and only populated when the
// resumeID is still present in the most recent session snapshot — fresh-after-
// reboot lookups still get the resumeID + size + modTime even when context is
// lost.
type TranscriptListEntry struct {
	ResumeID     string    `json:"resumeId"`
	Name         string    `json:"name,omitempty"`
	Type         string    `json:"type,omitempty"`
	SSHProfileID string    `json:"sshProfileId,omitempty"`
	Size         int64     `json:"size"`
	ModTime      time.Time `json:"modTime"`
}

// ListTranscripts enumerates stored transcripts and joins each one with the
// last-known terminal label. Label priority: active in-memory state > saved
// session snapshot > side-car metadata persisted at terminal create/rename.
// The side-car is the only source that survives closing a terminal, so
// closed-but-once-named sessions still show their label after a reboot.
func (a *App) ListTranscripts() ([]TranscriptListEntry, error) {
	entries, err := transcript.List()
	if err != nil {
		return nil, err
	}

	labels := map[string]session.TerminalState{}
	a.stateMu.Lock()
	for _, state := range a.activeTerminalStates {
		if state.ResumeID != "" {
			labels[state.ResumeID] = state
		}
	}
	a.stateMu.Unlock()

	if saved, err := session.LoadSession(); err == nil {
		for _, state := range saved.Terminals {
			if state.ResumeID == "" {
				continue
			}
			// Active state wins — it's the freshest source of truth.
			if _, ok := labels[state.ResumeID]; !ok {
				labels[state.ResumeID] = state
			}
		}
	}

	out := make([]TranscriptListEntry, 0, len(entries))
	for _, entry := range entries {
		listEntry := TranscriptListEntry{
			ResumeID: entry.ResumeID,
			Size:     entry.Size,
			ModTime:  entry.ModTime,
		}
		if label, ok := labels[entry.ResumeID]; ok {
			listEntry.Name = label.Name
			listEntry.Type = label.Type
			listEntry.SSHProfileID = label.SSHProfileID
		}
		// Side-car metadata fills in anything still unknown — that is the
		// path that catches old, closed terminals.
		if listEntry.Name == "" && entry.Metadata.Name != "" {
			listEntry.Name = entry.Metadata.Name
		}
		if listEntry.Type == "" && entry.Metadata.Type != "" {
			listEntry.Type = entry.Metadata.Type
		}
		if listEntry.SSHProfileID == "" && entry.Metadata.SSHProfileID != "" {
			listEntry.SSHProfileID = entry.Metadata.SSHProfileID
		}
		out = append(out, listEntry)
	}
	return out, nil
}

// SaveTranscriptMetadata writes a small JSON side-car next to the transcript
// so the terminal's label survives the session closing. Called by the
// frontend whenever a terminal is created or renamed. Empty resumeID is a
// no-op (template terminals without persistence shouldn't error here).
func (a *App) SaveTranscriptMetadata(resumeID, name, terminalType, sshProfileID string) error {
	if resumeID == "" {
		return nil
	}
	return transcript.WriteMetadata(resumeID, transcript.Metadata{
		Name:         name,
		Type:         terminalType,
		SSHProfileID: sshProfileID,
	})
}

// transcriptHardCap caps the bytes returned over IPC so a pathological 1 GiB
// transcript doesn't block the renderer. Negative or zero from the frontend
// means "as much as the cap allows".
const transcriptHardCap = 10 * 1024 * 1024

// GetTerminalTranscriptContent is the authoritative read endpoint. The result
// carries the file Size, the actually-read ReadBytes, and a Truncated flag —
// the frontend uses Truncated rather than guessing from string length, which
// is wrong for multi-byte UTF-8 content.
func (a *App) GetTerminalTranscriptContent(resumeID string, maxBytes int) (transcript.Content, error) {
	if maxBytes <= 0 || maxBytes > transcriptHardCap {
		maxBytes = transcriptHardCap
	}
	return transcript.ReadContent(resumeID, maxBytes)
}

// GetTerminalTranscriptFull returns just the text. Kept for the existing
// restore-transcript overlay path; new callers should prefer
// GetTerminalTranscriptContent so they can render truncation correctly.
func (a *App) GetTerminalTranscriptFull(resumeID string, maxBytes int) (string, error) {
	const hardCap = transcriptHardCap
	if maxBytes <= 0 || maxBytes > hardCap {
		maxBytes = hardCap
	}
	return transcript.ReadFull(resumeID, maxBytes)
}

// KillTmuxSession kills a tmux session by name. Used when a terminal is explicitly closed.
func (a *App) KillTmuxSession(sessionName string) error {
	if sessionName == "" {
		return nil
	}
	if runtime.GOOS == "windows" {
		return exec.Command("wsl.exe", "tmux", "-L", "mimir", "kill-session", "-t", sessionName).Run()
	}
	return exec.Command("tmux", "-L", "mimir", "kill-session", "-t", sessionName).Run()
}

// RemoveTerminalState is called by the frontend when a terminal is closed.
func (a *App) RemoveTerminalState(id int) {
	a.stateMu.Lock()
	defer a.stateMu.Unlock()
	delete(a.activeTerminalStates, id)
}

// ApplyTemplate applies a predefined template to the specified terminal.
func (a *App) ApplyTemplate(id int, templateName string, terminalType string) error {
	p, ok := a.TerminalManager.GetPty(id)
	if !ok {
		return fmt.Errorf("terminal with id %d not found", id)
	}

	ctx := a.getTemplateContext()
	err := a.TemplateManager.ApplyTemplate(id, templateName, terminalType, p, ctx)
	a.logToolExecution(activitylog.ToolExecutionEntry{
		Timestamp:    time.Now().Format(time.RFC3339),
		Source:       "user",
		ToolID:       "template:" + templateName,
		ToolName:     templateName,
		TerminalID:   id,
		TerminalType: terminalType,
		Output:       "Applied template",
		Error:        errorString(err),
	})
	return err
}

// ApplyTemplateWithVariables applies a template with additional user-provided variables.
func (a *App) ApplyTemplateWithVariables(id int, templateName string, terminalType string, variables map[string]string) error {
	p, ok := a.TerminalManager.GetPty(id)
	if !ok {
		return fmt.Errorf("terminal with id %d not found", id)
	}

	ctx := a.getTemplateContext()
	ctx.Variables = variables
	err := a.TemplateManager.ApplyTemplate(id, templateName, terminalType, p, ctx)
	a.logToolExecution(activitylog.ToolExecutionEntry{
		Timestamp:    time.Now().Format(time.RFC3339),
		Source:       "user",
		ToolID:       "template:" + templateName,
		ToolName:     templateName,
		TerminalID:   id,
		TerminalType: terminalType,
		Inputs:       variables,
		Output:       "Applied template with variables",
		Error:        errorString(err),
	})
	return err
}

// Delegate methods to TerminalManager

func (a *App) StartTerminal(terminalType string) (int, error) {
	return a.StartTerminalWithOptions(terminalType, "")
}

func (a *App) StartTerminalWithOptions(terminalType string, tmuxSessionName string) (int, error) {
	id, err := a.TerminalManager.StartTerminalWithOptions(terminalType, tmuxSessionName)
	if err == nil {
		a.TerminalManager.SetSessionMeta(id, terminalType, "")
	}
	return id, err
}

func (a *App) GetTerminalTmuxStatus(terminalID int) map[string]any {
	if meta := a.TerminalManager.GetSSHMeta(terminalID); meta != nil {
		return map[string]any{
			"active":      meta.Config.TmuxActive,
			"sessionName": meta.Config.TmuxSessionName,
			"mode":        meta.Config.TmuxMode,
			"status":      meta.Config.TmuxStatus,
			"error":       meta.Config.TmuxError,
		}
	}
	meta := a.TerminalManager.GetTerminalRuntimeMeta(terminalID)
	return map[string]any{
		"active":      meta.TmuxActive,
		"sessionName": meta.TmuxSessionName,
		"mode":        meta.TmuxMode,
		"status":      meta.TmuxStatus,
		"error":       meta.TmuxError,
		"shellPath":   meta.ShellPath,
	}
}

func (a *App) ConfirmFrontendReady(id int) error {
	return a.TerminalManager.ConfirmFrontendReady(id)
}

func (a *App) InitializeTerminal(id int) error {
	return a.TerminalManager.InitializeTerminal(id)
}

func (a *App) WriteToTerminal(id int, data string) error {
	return a.TerminalManager.WriteToTerminal(id, data)
}

func (a *App) ResizeTerminal(id int, rowsStr string, colsStr string) error {
	return a.TerminalManager.ResizeTerminal(id, rowsStr, colsStr)
}

func (a *App) CloseTerminal(id int) error {
	return a.TerminalManager.CloseTerminal(id)
}

// Delegate methods to TemplateManager

func (a *App) GetTemplates() ([]template.Template, error) {
	return a.TemplateManager.GetTemplates()
}

func (a *App) ReloadTemplates() ([]template.Template, error) {
	templates, err := a.TemplateManager.ReloadTemplates()
	if err == nil {
		a.invalidateFunctionCatalog()
	}
	return templates, err
}

func (a *App) SaveTemplate(templateJSON string) ([]template.Template, error) {
	templates, err := a.TemplateManager.SaveTemplate(templateJSON)
	if err == nil {
		a.invalidateFunctionCatalog()
	}
	return templates, err
}

func (a *App) UpdateTemplate(templateJSON string) ([]template.Template, error) {
	templates, err := a.TemplateManager.UpdateTemplate(templateJSON)
	if err == nil {
		a.invalidateFunctionCatalog()
	}
	return templates, err
}

func (a *App) DeleteTemplate(templateName string) ([]template.Template, error) {
	templates, err := a.TemplateManager.DeleteTemplate(templateName)
	if err == nil {
		a.invalidateFunctionCatalog()
	}
	return templates, err
}

func (a *App) ToggleFavorite(templateName string) ([]template.Template, error) {
	templates, err := a.TemplateManager.ToggleFavorite(templateName)
	if err == nil {
		a.invalidateFunctionCatalog()
	}
	return templates, err
}

// FileInfo represents information about a file or directory.
type FileInfo struct {
	Name    string `json:"name"`
	IsDir   bool   `json:"isDir"`
	Size    int64  `json:"size"`
	ModTime int64  `json:"modTime"`
}

// TerminalTypeOption describes a terminal type the current platform can start.
type TerminalTypeOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

func executableAvailable(names []string, fallbackPaths []string) bool {
	for _, path := range fallbackPaths {
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() && info.Mode()&0111 != 0 {
			return true
		}
	}
	for _, name := range names {
		if _, err := exec.LookPath(name); err == nil {
			return true
		}
	}
	return false
}

// GetAvailableTerminalTypes returns terminal types that are expected to work on this system.
func (a *App) GetAvailableTerminalTypes() []TerminalTypeOption {
	if runtime.GOOS == "windows" {
		options := []TerminalTypeOption{
			{Value: "cmd", Label: "CMD"},
			{Value: "powershell", Label: "PowerShell"},
		}
		if executableAvailable([]string{"wsl.exe"}, nil) {
			options = append(options, TerminalTypeOption{Value: "wsl", Label: "WSL"})
		}
		if executableAvailable([]string{"bash.exe", "bash"}, nil) {
			options = append(options, TerminalTypeOption{Value: "bash", Label: "Bash"})
		}
		if executableAvailable([]string{"zsh.exe", "zsh"}, nil) {
			options = append(options, TerminalTypeOption{Value: "zsh", Label: "Zsh"})
		}
		options = append(options, TerminalTypeOption{Value: "ssh", Label: "SSH"})
		return options
	}

	options := []TerminalTypeOption{}
	if executableAvailable(
		[]string{"bash"},
		[]string{"/bin/bash", "/usr/bin/bash", "/usr/local/bin/bash"},
	) {
		options = append(options, TerminalTypeOption{Value: "bash", Label: "Bash"})
	} else if executableAvailable(
		[]string{"sh"},
		[]string{"/bin/sh", "/usr/bin/sh"},
	) {
		options = append(options, TerminalTypeOption{Value: "bash", Label: "Shell"})
	}
	if executableAvailable(
		[]string{"zsh"},
		[]string{"/bin/zsh", "/usr/bin/zsh", "/usr/local/bin/zsh"},
	) {
		options = append(options, TerminalTypeOption{Value: "zsh", Label: "Zsh"})
	}
	options = append(options, TerminalTypeOption{Value: "ssh", Label: "SSH"})
	return options
}

// isValidPath validates if a path is safe to access.
func isValidPath(path string) bool {
	if path == "" {
		return false
	}
	// Check for path traversal attempts
	if strings.Contains(path, "..") {
		return false
	}
	// Convert to absolute path and clean it
	cleanPath := filepath.Clean(path)
	// Check if it's an absolute path (for security)
	if !filepath.IsAbs(cleanPath) {
		return false
	}
	return true
}

// ListDirectory lists the contents of a directory.
func (a *App) ListDirectory(path string) ([]FileInfo, error) {
	if !isValidPath(path) {
		a.logSecurityEvent(activitylog.SecurityEventEntry{
			Timestamp: time.Now().Format(time.RFC3339),
			Event:     "invalid_path",
			Operation: "list_directory",
			Path:      path,
			Reason:    "path validation failed",
		})
		return nil, fmt.Errorf("invalid path: %s", path)
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	var files []FileInfo
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, FileInfo{
			Name:    entry.Name(),
			IsDir:   entry.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime().Unix(),
		})
	}
	return files, nil
}

// GetFileContent reads the content of a text file.
func (a *App) GetFileContent(path string) (string, error) {
	if !isValidPath(path) {
		a.logSecurityEvent(activitylog.SecurityEventEntry{
			Timestamp: time.Now().Format(time.RFC3339),
			Event:     "invalid_path",
			Operation: "get_file_content",
			Path:      path,
			Reason:    "path validation failed",
		})
		return "", fmt.Errorf("invalid path: %s", path)
	}

	// Limit file size to prevent reading very large files
	const maxFileSize = 1024 * 1024 // 1 MB

	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("failed to get file info: %w", err)
	}

	if info.IsDir() {
		return "", fmt.Errorf("path is a directory, not a file")
	}

	if info.Size() > maxFileSize {
		return "", fmt.Errorf("file size exceeds limit (%d bytes)", maxFileSize)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read file content: %w", err)
	}

	return string(content), nil
}

// OpenPathInExplorer opens a given path in the system's file explorer.
func (a *App) OpenPathInExplorer(path string) error {
	if !isValidPath(path) {
		a.logSecurityEvent(activitylog.SecurityEventEntry{
			Timestamp: time.Now().Format(time.RFC3339),
			Event:     "invalid_path",
			Operation: "open_path_in_explorer",
			Path:      path,
			Reason:    "path validation failed",
		})
		return fmt.Errorf("invalid path: %s", path)
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", path)
	case "darwin":
		cmd = exec.Command("open", path)
	case "linux":
		cmd = exec.Command("xdg-open", path)
	default:
		return fmt.Errorf("unsupported platform")
	}

	err := cmd.Start()
	if err != nil {
		a.logSecurityEvent(activitylog.SecurityEventEntry{
			Timestamp: time.Now().Format(time.RFC3339),
			Event:     "open_path_failed",
			Operation: "open_path_in_explorer",
			Path:      path,
			Reason:    err.Error(),
		})
	}
	return err
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (a *App) logSecurityEvent(entry activitylog.SecurityEventEntry) {
	_ = activitylog.Append(activitylog.KindSecurityEvents, entry)
}

func (a *App) logToolExecution(entry activitylog.ToolExecutionEntry) {
	_ = activitylog.Append(activitylog.KindToolExecutions, entry)
}
