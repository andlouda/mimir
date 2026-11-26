package terminal

import (
	"os"
	"path/filepath"

	"mimir/appdir"
)

// AgentDetectionDisabledFile marks that the user turned agent detection off
// (Settings). Its absence means detection is on, the default.
const AgentDetectionDisabledFile = "agent_detection_disabled"

// agentDetectionEnabled reports whether coding-agent detection is active. It
// counts as consent for the lightweight prompt hook: on terminals without
// tmux (PowerShell, cmd, tmux mode "off") the hook's cwd beacon is the only
// way to learn which project the agent runs in.
func agentDetectionEnabled() bool {
	appHome, err := appdir.Home()
	if err != nil {
		return true
	}
	_, err = os.Stat(filepath.Join(appHome, AgentDetectionDisabledFile))
	return err != nil
}
