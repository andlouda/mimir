package main

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"mimir/executil"
	"mimir/ssh"
)

// The session overview answers "what is still running, and where": every
// tmux session on the hosts Mimir is connected to and on Mimir's local tmux
// server, with the program in it and whether a pane shows it. Sessions that
// lost their pane (closed connection, crashed app) stay findable and can be
// opened again or ended.
//
// It is read-only except for two explicit user actions: opening a session
// in a pane and ending one. Hosts are asked over a connection a pane already
// holds, so listing never logs in anywhere. Sessions Mimir did not create
// are listed but can neither be opened nor ended from here.

// tmuxSessionRow is one tmux session.
type tmuxSessionRow struct {
	Name     string `json:"name"`
	Attached int    `json:"attached"` // tmux clients showing the session
	Created  int64  `json:"created"`  // unix seconds
	Windows  int    `json:"windows"`
	Command  string `json:"command"` // program in the active pane
	Path     string `json:"path"`    // its working directory
	// Mimir is true when Mimir created the session (for this profile, or on
	// its own local socket); only those may be opened or ended.
	Mimir bool `json:"mimir"`
	// TerminalID is the Mimir pane attached to the session, 0 when none.
	TerminalID int `json:"terminalId"`
}

// tmuxSessionGroup is one place sessions live in.
type tmuxSessionGroup struct {
	Kind         string           `json:"kind"` // "ssh" or "local"
	Label        string           `json:"label"`
	ProfileID    string           `json:"profileId,omitempty"`
	TerminalID   int              `json:"terminalId,omitempty"` // pane whose connection answered
	TerminalType string           `json:"terminalType,omitempty"`
	Error        string           `json:"error,omitempty"`
	Sessions     []tmuxSessionRow `json:"sessions"`
}

const tmuxSessionFieldSep = "\t"

// tmuxSessionListFormat prints one line per pane; the active pane of the
// active window carries "11" in the fifth field.
const tmuxSessionListFormat = "#{session_name}" + tmuxSessionFieldSep + "#{session_attached}" + tmuxSessionFieldSep +
	"#{session_created}" + tmuxSessionFieldSep + "#{session_windows}" + tmuxSessionFieldSep +
	"#{window_active}#{pane_active}" + tmuxSessionFieldSep + "#{pane_current_command}" + tmuxSessionFieldSep + "#{pane_current_path}"

// parseTmuxSessionList folds the per-pane lines into one row per session,
// taking program and directory from the session's active pane.
func parseTmuxSessionList(output string) []tmuxSessionRow {
	byName := map[string]*tmuxSessionRow{}
	var order []string
	for _, line := range strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n") {
		fields := strings.SplitN(line, tmuxSessionFieldSep, 7)
		if len(fields) < 7 || fields[0] == "" {
			continue
		}
		row, seen := byName[fields[0]]
		if !seen {
			row = &tmuxSessionRow{Name: fields[0]}
			row.Attached, _ = strconv.Atoi(fields[1])
			row.Created, _ = strconv.ParseInt(fields[2], 10, 64)
			row.Windows, _ = strconv.Atoi(fields[3])
			byName[fields[0]] = row
			order = append(order, fields[0])
		}
		if !seen || fields[4] == "11" {
			row.Command, row.Path = fields[5], fields[6]
		}
	}
	rows := make([]tmuxSessionRow, 0, len(order))
	for _, name := range order {
		rows = append(rows, *byName[name])
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Created > rows[j].Created })
	return rows
}

// ListTmuxSessionsJSON returns the session overview as JSON groups: one per
// SSH profile with a connected pane, one for Mimir's local tmux server.
func (a *App) ListTmuxSessionsJSON() (string, error) {
	groups := a.sshTmuxSessionGroups()
	if local := a.localTmuxSessionGroup(); local != nil {
		groups = append(groups, *local)
	}
	data, err := json.Marshal(groups)
	if err != nil {
		return "", fmt.Errorf("encode tmux sessions: %w", err)
	}
	return string(data), nil
}

func (a *App) sshTmuxSessionGroups() []tmuxSessionGroup {
	ids := a.TerminalManager.SessionIDs()
	sort.Ints(ids)
	// Which pane shows which session, and one live connection per profile.
	paneOf := map[string]int{} // profileID + "\x00" + session → terminal id
	connOf := map[string]int{} // profileID → terminal id with a connection
	var profiles []string
	for _, id := range ids {
		meta := a.TerminalManager.GetSSHMeta(id)
		if meta == nil {
			continue
		}
		if name := strings.TrimSpace(meta.Config.TmuxSessionName); name != "" && meta.Config.TmuxActive {
			paneOf[meta.ProfileID+"\x00"+name] = id
		}
		if _, known := connOf[meta.ProfileID]; !known && a.TerminalManager.GetSSHClient(id) != nil {
			connOf[meta.ProfileID] = id
			profiles = append(profiles, meta.ProfileID)
		}
	}
	groups := make([]tmuxSessionGroup, 0, len(profiles))
	for _, profileID := range profiles {
		id := connOf[profileID]
		group := tmuxSessionGroup{Kind: "ssh", ProfileID: profileID, TerminalID: id, TerminalType: "ssh", Label: profileID, Sessions: []tmuxSessionRow{}}
		if a.sshProfileStore != nil {
			if p, ok := a.sshProfileStore.Get(profileID); ok {
				group.Label = p.Name
			}
		}
		client := a.TerminalManager.GetSSHClient(id)
		if client == nil {
			group.Error = "no connection"
			groups = append(groups, group)
			continue
		}
		// A host without a running tmux server answers with an error and
		// no lines: an empty list, not a failure.
		out, _ := ssh.RunCommand(client, "tmux list-panes -a -F "+executil.ShellQuote(tmuxSessionListFormat)+" 2>/dev/null; true", discoveryTimeout)
		for _, row := range parseTmuxSessionList(out) {
			row.Mimir = reusableSSHTmuxName(profileID, row.Name)
			row.TerminalID = paneOf[profileID+"\x00"+row.Name]
			group.Sessions = append(group.Sessions, row)
		}
		groups = append(groups, group)
	}
	return groups
}

// localTmuxTerminalType is the pane type that reaches Mimir's local tmux
// server: the WSL distro on Windows, a plain shell elsewhere.
func localTmuxTerminalType() string {
	if runtime.GOOS == "windows" {
		return "wsl"
	}
	return "bash"
}

func (a *App) localTmuxSessionGroup() *tmuxSessionGroup {
	ctx, cancel := context.WithTimeout(context.Background(), discoveryTimeout)
	defer cancel()
	out, err := executil.TmuxCommand(ctx, runtime.GOOS == "windows", "list-panes", "-a", "-F", tmuxSessionListFormat).Output()
	rows := parseTmuxSessionList(string(out))
	if err != nil && len(rows) == 0 {
		// No tmux, no WSL or no server running: nothing to show.
		return nil
	}
	paneOf := map[string]int{}
	for _, id := range a.TerminalManager.SessionIDs() {
		if meta := a.TerminalManager.GetTerminalRuntimeMeta(id); meta.TmuxActive && meta.TmuxSessionName != "" {
			paneOf[meta.TmuxSessionName] = id
		}
	}
	group := &tmuxSessionGroup{Kind: "local", Label: "local", TerminalType: localTmuxTerminalType(), Sessions: []tmuxSessionRow{}}
	for _, row := range rows {
		// Everything on Mimir's own socket was created by Mimir.
		row.Mimir = executil.ValidTmuxSessionName(row.Name)
		row.TerminalID = paneOf[row.Name]
		group.Sessions = append(group.Sessions, row)
	}
	return group
}

// KillSSHTmuxSessionByName ends a session from the overview on the host of
// the given pane. Like closing a pane it only ends sessions Mimir created
// for that pane's profile; a session a pane is showing is ended by closing
// the pane instead.
func (a *App) KillSSHTmuxSessionByName(terminalID int, sessionName string) error {
	meta := a.TerminalManager.GetSSHMeta(terminalID)
	if meta == nil {
		return fmt.Errorf("terminal %d is not an SSH pane", terminalID)
	}
	sessionName = strings.TrimSpace(sessionName)
	if !reusableSSHTmuxName(meta.ProfileID, sessionName) {
		return fmt.Errorf("tmux session %q was not created by Mimir for this profile", sessionName)
	}
	for _, id := range a.TerminalManager.SessionIDs() {
		if other := a.TerminalManager.GetSSHMeta(id); other != nil && other.ProfileID == meta.ProfileID &&
			other.Config.TmuxActive && other.Config.TmuxSessionName == sessionName && a.TerminalManager.GetSSHClient(id) != nil {
			return fmt.Errorf("tmux session %s is open in a pane; close the pane to end it", sessionName)
		}
	}
	client := a.TerminalManager.GetSSHClient(terminalID)
	if client == nil {
		return fmt.Errorf("no connection to end tmux session %s on the host", sessionName)
	}
	_, _ = ssh.RunCommand(client, sshKillSessionCommand(sessionName)+" 2>/dev/null; true", discoveryTimeout)
	return nil
}
