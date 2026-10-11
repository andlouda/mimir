// Package agentsvc is the app-side integration of the AI agents that run in
// Mimir's panes: detecting them, watching their state and permission
// prompts, installing Claude Code's hooks, serving their transcripts and
// timeline, and the workspace that groups their sessions. The App forwards
// the Wails bindings here and otherwise stays out of it.
package agentsvc

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/pkg/sftp"

	"mimir/executil"
	"mimir/ssh"
	"mimir/terminal"
	"mimir/timeline"
)

// RemoteFiles is the SFTP surface the service needs on an SSH pane's host.
type RemoteFiles interface {
	ReadDir(path string) ([]os.FileInfo, error)
	Stat(path string) (os.FileInfo, error)
	Open(path string) (*sftp.File, error)
	OpenFile(path string, flags int) (*sftp.File, error)
	Remove(path string) error
	Getwd() (string, error)
	Close() error
}

// Deps is what the service borrows from the app. Timeline and SSHProfiles
// may be nil; the matching features then stay off.
type Deps struct {
	Terminals   *terminal.Manager
	Timeline    *timeline.Store
	SSHProfiles *ssh.ProfileStore
	// Settings returns the current agent timeline settings.
	Settings func() timeline.Settings
	// RemoteFiles opens an SFTP client on the host of an SSH terminal.
	RemoteFiles func(terminalID int) (RemoteFiles, error)
	// OpenPath shows a folder in the desktop's file manager.
	OpenPath func(path string) error
}

// Service holds the per-pane agent state. One instance per app. The
// lifecycle entry points (Start, StopWatcher, UpgradeOutdatedHooks,
// RunPaneScript) tolerate a nil receiver so an App built without the
// service (tests) still works.
type Service struct {
	deps            Deps
	terminals       *terminal.Manager
	timeline        *timeline.Store
	sshProfileStore *ssh.ProfileStore
	// ctx is the Wails context for event emission; nil until Start.
	ctx context.Context

	agentStates  map[int]agentTerminalState
	agentPrompts map[int]agentPrompt
	// agentAnswered: arrival time of the prompt answered per pane, see
	// markPromptAnswered.
	agentAnswered map[int]time.Time
	agentWatchers map[int]*agentWatcher
	// agentDetectReasons: last logged "nothing found" reason per pane.
	agentDetectReasons map[int]string
	agentMu            sync.Mutex
}

// New wires the service; nothing runs until Start.
func New(d Deps) *Service {
	return &Service{deps: d, terminals: d.Terminals, timeline: d.Timeline, sshProfileStore: d.SSHProfiles}
}

// Start hands over the Wails context and begins forwarding timeline
// events to the frontend.
func (a *Service) Start(ctx context.Context) {
	if a == nil {
		return
	}
	a.ctx = ctx
	a.startTimelineEmitter()
	go a.scanUnboundHookEvents(ctx)
}

// telemetrySettings is the current copy of the timeline settings; all off
// when the app provides none (tests).
func (a *Service) telemetrySettings() timeline.Settings {
	if a.deps.Settings == nil {
		return timeline.Settings{}
	}
	return a.deps.Settings()
}

// hookToolEventsWanted says whether the installed hook set should include
// the tool events.
func (a *Service) hookToolEventsWanted() bool {
	s := a.telemetrySettings()
	return s.Enabled && s.HookToolEvents
}

func (a *Service) remoteFiles(terminalID int) (RemoteFiles, error) {
	if a.deps.RemoteFiles == nil {
		return nil, fmt.Errorf("no remote file access")
	}
	return a.deps.RemoteFiles(terminalID)
}

// discoveryTimeout bounds the helper execs and remote probes.
const discoveryTimeout = executil.HelperTimeout
