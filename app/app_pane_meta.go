package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"mimir/executil"
	"mimir/ssh"
)

// Pane placement on the host.
//
// Mimir's layout lives in one local file. When that file is lost, the tmux
// sessions keep running on the hosts, but which pane showed which session,
// under what name and where in the window is gone.
//
// For SSH profiles that opt in (Profile.LayoutOnHost), each pane's name and
// its place in the split tree are therefore also kept with the tmux session
// it shows, as the tmux user option @mimir-pane. No file is written on the
// host: the option lives inside the tmux server and disappears with the
// session. The session overview reads it back, so a host's panes can return
// with their names and arrangement.
//
// Off by default: nothing is stored on a host unless its profile asks for
// it. Only sessions Mimir created for that profile are ever written to.

const (
	paneMetaOption  = "@mimir-pane"
	paneMetaVersion = 1
	// paneMetaMaxBytes bounds the stored value; a split path of ordinary
	// depth and a name need a few hundred bytes.
	paneMetaMaxBytes = 1500
	paneMetaMaxName  = 80
	paneMetaMaxDepth = 24
)

// paneMetaStep is one split on the way from the window's root to the pane.
type paneMetaStep struct {
	Direction string  `json:"d"` // "h" or "v"
	Ratio     float64 `json:"r"` // share of the first child
	Index     int     `json:"i"` // 0: first child, 1: second
}

// paneMeta is what a session remembers about the pane that showed it.
type paneMeta struct {
	Version int            `json:"v"`
	Name    string         `json:"name,omitempty"`
	Path    []paneMetaStep `json:"path"`
}

// parsePaneMeta validates a stored or submitted value. Anything that is not
// exactly the expected shape is rejected: the value comes back from a host
// and must not be trusted further than this.
func parsePaneMeta(raw string) (paneMeta, bool) {
	if len(raw) > paneMetaMaxBytes {
		return paneMeta{}, false
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return paneMeta{}, false
	}
	var meta paneMeta
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&meta); err != nil {
		return paneMeta{}, false
	}
	if meta.Version != paneMetaVersion || len(meta.Path) > paneMetaMaxDepth || len([]rune(meta.Name)) > paneMetaMaxName {
		return paneMeta{}, false
	}
	for _, r := range meta.Name {
		if r < 0x20 || r == 0x7f {
			return paneMeta{}, false
		}
	}
	for _, step := range meta.Path {
		if step.Direction != "h" && step.Direction != "v" {
			return paneMeta{}, false
		}
		if step.Index != 0 && step.Index != 1 {
			return paneMeta{}, false
		}
		if !(step.Ratio >= 0.1 && step.Ratio <= 0.9) {
			return paneMeta{}, false
		}
	}
	if meta.Path == nil {
		meta.Path = []paneMetaStep{}
	}
	return meta, true
}

// sshLayoutOnHost reports whether the profile opted into keeping pane
// placement on the host.
func (a *App) sshLayoutOnHost(profileID string) bool {
	if a.sshProfileStore == nil {
		return false
	}
	profile, ok := a.sshProfileStore.Get(profileID)
	return ok && profile.LayoutOnHost
}

// paneMetaSetCommand stores value on the session. The target ends in ":" so
// tmux reads "=name" as an exact session name; without it set-option looks
// for a pane and fails.
func paneMetaSetCommand(sessionName, value string) string {
	return "tmux set-option -t " + executil.ShellQuote("="+sessionName+":") + " " + paneMetaOption + " " + executil.ShellQuote(value)
}

// SaveSSHPaneMeta stores the pane's name and place on the tmux session it
// shows. It refuses unless the pane's profile opted in and the session is
// one Mimir created for that profile.
func (a *App) SaveSSHPaneMeta(terminalID int, metaJSON string) error {
	sshMeta := a.TerminalManager.GetSSHMeta(terminalID)
	if sshMeta == nil {
		return fmt.Errorf("terminal %d is not an SSH pane", terminalID)
	}
	if !a.sshLayoutOnHost(sshMeta.ProfileID) {
		return fmt.Errorf("the SSH profile does not keep pane placement on the host")
	}
	sessionName := strings.TrimSpace(sshMeta.Config.TmuxSessionName)
	if !sshMeta.Config.TmuxActive || !reusableSSHTmuxName(sshMeta.ProfileID, sessionName) {
		return fmt.Errorf("the pane has no tmux session created by Mimir")
	}
	meta, ok := parsePaneMeta(metaJSON)
	if !ok {
		return fmt.Errorf("invalid pane placement")
	}
	value, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("encode pane placement: %w", err)
	}
	client := a.TerminalManager.GetSSHClient(terminalID)
	if client == nil {
		return fmt.Errorf("no connection to store the pane placement")
	}
	if _, err := ssh.RunCommand(client, paneMetaSetCommand(sessionName, string(value))+" 2>/dev/null", discoveryTimeout); err != nil {
		return fmt.Errorf("store pane placement on the host: %w", err)
	}
	return nil
}
