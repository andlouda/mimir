package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"mimir/activitylog"
	"mimir/aiflow"

	gossh "golang.org/x/crypto/ssh"
)

const discoveryTimeout = 10 * time.Second

// RunDiscoveryForTerminalJSON resolves discovery values in the context of a
// specific terminal:
//   - SSH terminals: the allowlisted discovery command runs on the remote host
//     over the existing SSH connection, in the directory of the remote tmux pane.
//   - Local tmux terminals (bash/zsh/wsl): the command runs locally in the
//     pane's current directory.
//   - Anything else falls back to the legacy behavior (app working directory).
func (a *App) RunDiscoveryForTerminalJSON(terminalID int, discoveryTool string, terminalType string, variablesJSON string) (string, error) {
	var variables map[string]string
	if strings.TrimSpace(variablesJSON) != "" {
		if err := json.Unmarshal([]byte(variablesJSON), &variables); err != nil {
			return "", fmt.Errorf("failed to parse discovery variables: %w", err)
		}
	}
	if variables == nil {
		variables = map[string]string{}
	}

	if client := a.TerminalManager.GetSSHClient(terminalID); client != nil {
		values, err := a.runRemoteDiscovery(client, terminalID, discoveryTool, variables)
		if err != nil {
			return "", err
		}
		return marshalDiscoveryValues(values)
	}

	workdir := a.localTerminalCwd(terminalID, terminalType)
	if workdir == "" {
		workdir = a.getTemplateContext().CurrentDir
	}
	resolver := aiflow.NewCachedDiscoveryResolver(workdir)
	values, err := resolver.Resolve(discoveryTool, terminalType, variables)
	if err != nil {
		return "", err
	}
	return marshalDiscoveryValues(values)
}

func (a *App) runRemoteDiscovery(client *gossh.Client, terminalID int, discoveryTool string, variables map[string]string) ([]string, error) {
	tmuxSession := ""
	if meta := a.TerminalManager.GetSSHMeta(terminalID); meta != nil {
		tmuxSession = meta.Config.TmuxSessionName
	}

	script, err := aiflow.BuildRemoteDiscoveryScript(discoveryTool, variables, tmuxSession)
	if err != nil {
		logRemoteDiscoveryEvent("discovery_denied", discoveryTool, err.Error())
		return nil, err
	}

	output, err := runSSHCommandWithTimeout(client, script, discoveryTimeout)
	if err != nil {
		message := strings.TrimSpace(output)
		if message == "" {
			message = err.Error()
		}
		logRemoteDiscoveryEvent("discovery_failed", discoveryTool, message)
		return nil, fmt.Errorf("remote discovery %s failed: %s", discoveryTool, firstOutputLine(message))
	}

	values := aiflow.NormalizeDiscoveryOutput(output)
	logRemoteDiscoveryEvent("discovery_resolved", discoveryTool, fmt.Sprintf("%d values", len(values)))
	return values, nil
}

// runSSHCommandWithTimeout executes a command in a fresh exec session on the
// existing SSH connection. No new login or credentials are involved.
func runSSHCommandWithTimeout(client *gossh.Client, command string, timeout time.Duration) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("ssh session failed: %w", err)
	}
	defer session.Close()

	type result struct {
		output []byte
		err    error
	}
	done := make(chan result, 1)
	go func() {
		output, err := session.CombinedOutput(command)
		done <- result{output, err}
	}()

	select {
	case r := <-done:
		return string(r.output), r.err
	case <-time.After(timeout):
		// Close unblocks the CombinedOutput goroutine.
		session.Close()
		return "", fmt.Errorf("remote command timed out after %s", timeout)
	}
}

// localTerminalCwd resolves the current working directory of a local terminal
// by asking its tmux server for the pane's path. Works without any shell
// integration; returns "" when the terminal has no tmux session.
func (a *App) localTerminalCwd(terminalID int, terminalType string) string {
	meta := a.TerminalManager.GetTerminalRuntimeMeta(terminalID)
	if !meta.TmuxActive || strings.TrimSpace(meta.TmuxSessionName) == "" {
		return ""
	}
	target := strings.TrimSpace(meta.TmuxSessionName) + ":"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	switch strings.TrimSpace(strings.ToLower(terminalType)) {
	case "wsl":
		cmd = exec.CommandContext(ctx, "wsl.exe", "--", "tmux", "-L", "mimir", "display-message", "-p", "-t", target, "#{pane_current_path}")
	case "bash", "zsh":
		cmd = exec.CommandContext(ctx, "tmux", "-L", "mimir", "display-message", "-p", "-t", target, "#{pane_current_path}")
	default:
		return ""
	}

	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	cwd := strings.TrimSpace(string(output))
	if !strings.HasPrefix(cwd, "/") {
		return ""
	}
	return cwd
}

func marshalDiscoveryValues(values []string) (string, error) {
	payload, err := json.Marshal(values)
	if err != nil {
		return "", fmt.Errorf("failed to encode discovery result: %w", err)
	}
	return string(payload), nil
}

func firstOutputLine(message string) string {
	if index := strings.IndexByte(message, '\n'); index >= 0 {
		return strings.TrimSpace(message[:index])
	}
	return message
}

func logRemoteDiscoveryEvent(event string, discoveryTool string, reason string) {
	_ = activitylog.Append(activitylog.KindSecurityEvents, activitylog.SecurityEventEntry{
		Timestamp: time.Now().Format(time.RFC3339),
		Event:     event,
		Operation: "ai_discovery_remote",
		Reason:    reason,
		Metadata: map[string]string{
			"discoveryTool": discoveryTool,
		},
	})
}
