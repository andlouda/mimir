# Reboot Workspace Restore

Status: Feature concept / high priority
Last updated: 2026-06-06

## Goal

Mimir should help admins recover their working context after a machine reboot,
app restart, crash, or maintenance window.

The goal is not to keep processes alive across reboot. That is impossible for
normal terminal processes. The goal is to restore the workspace:

- terminal layout
- session names
- shell types
- working directories
- SSH targets
- tmux session names where available
- recent transcript/scrollback
- command history
- last known running command
- safe "run again" affordances

This is especially important for Windows and Linux admins during patch cycles,
host restarts, incident response, and long troubleshooting sessions.

## Core Distinction

There are two different persistence problems:

| Problem | Meaning | Possible? | Mechanism |
| --- | --- | --- | --- |
| Runtime persistence | Mimir closes, shell keeps running | Yes | tmux on Unix/WSL/SSH, Windows ConPTY agent for native Windows |
| Reboot restore | Machine reboots, workspace returns | Partially | Snapshot + restore UI + reconnect/re-run options |

No implementation can keep a normal shell process alive through a real reboot.
After reboot, the process is gone. Mimir can only reconstruct context and offer
safe recovery actions.

## MVP User Story

An admin is debugging a service failure with several terminals open:

- local PowerShell
- SSH to a Linux server
- Docker logs
- Kubernetes triage playbook
- notes and command history

After a Windows or Linux reboot, Mimir starts and shows a restore screen:

- previous workspace name and timestamp
- all previous terminals and panes
- which sessions can reconnect automatically
- which sessions are gone and can be recreated
- last transcript for each terminal
- last commands with "copy" / "run again" actions

The admin can restore the workspace without remembering every host, path, and
command.

## What Can Be Restored

### Always Restorable

- Terminal layout tree
- Terminal display names
- Shell type (`bash`, `zsh`, `wsl`, `cmd`, `powershell`, `ssh`)
- Last known CWD when known
- SSH profile ID / host label
- tmux session name
- transcript excerpt
- command history entries
- notes panel state
- active app page
- workflow/playbook draft
- last workflow run summary

### Conditionally Restorable

- SSH session: reconnect if host is reachable
- SSH tmux session: reattach if remote tmux session survived
- WSL tmux session: reattach if WSL was not shut down
- local tmux session: reattach if Linux was not rebooted
- native Windows shell: reattach only if a future ConPTY agent kept it alive

### Not Restorable As The Same Process

- local shell after reboot
- PowerShell/cmd process after reboot
- foreground command after reboot
- `tail -f`, `kubectl logs -f`, package installs, scripts, REPLs
- SSH connection after local or remote reboot

These can only be recreated or re-run with explicit user action.

## Proposed Architecture

### 1. Workspace Snapshot Store

Add a persistent snapshot store, likely under:

```text
UserConfigDir/mimir/workspaces/
```

Possible files:

```text
workspaces/
├── latest.json
├── snapshots/
│   ├── 2026-06-06T10-12-30Z.json
│   └── 2026-06-06T11-55-01Z.json
└── transcripts/
    └── <resume-id>.txt
```

Snapshot shape:

```json
{
  "schemaVersion": 1,
  "snapshotId": "2026-06-06T10-12-30Z",
  "createdAt": "2026-06-06T10:12:30Z",
  "appVersion": "0.2.0",
  "activePage": "terminals",
  "layoutTree": {},
  "terminals": [
    {
      "id": 1,
      "resumeId": "local-bash-abc",
      "name": "API host",
      "type": "ssh",
      "cwd": "/var/log",
      "sshProfileId": "prod-api",
      "tmuxSessionName": "mimir-prod-api",
      "tmuxActive": true,
      "lastCommand": "journalctl -u api.service -n 100",
      "lastExitCode": "0",
      "transcriptPath": "transcripts/local-bash-abc.txt",
      "restoreMode": "reconnect"
    }
  ],
  "workflowDraft": {},
  "notesOpen": true
}
```

Writes should use `safeio.AtomicWriteFile`.

### 2. Snapshot Timing

Create/update snapshots on:

- app startup after restore completes
- terminal create/close/split/minimize
- terminal rename
- CWD update where known
- command history event
- workflow draft change
- app close
- periodic timer, e.g. every 30-60 seconds

Avoid writing on every output chunk. Transcript persistence already handles raw
output separately.

### 3. Restore Planner

On startup, compare the saved snapshot with current runtime state and produce a
restore plan:

```json
{
  "snapshotId": "...",
  "items": [
    {
      "resumeId": "local-bash-abc",
      "label": "API host",
      "type": "ssh",
      "status": "can_reconnect_tmux",
      "action": "reattach"
    },
    {
      "resumeId": "powershell-xyz",
      "label": "Local PowerShell",
      "type": "powershell",
      "status": "process_gone",
      "action": "recreate_shell"
    }
  ]
}
```

Status examples:

- `can_reattach`
- `can_reconnect_tmux`
- `can_recreate_shell`
- `process_gone`
- `host_unreachable`
- `unsupported`
- `needs_user_confirmation`

### 4. Restore UI

Add a first-run-after-reboot restore surface:

- "Restore previous workspace"
- "Start fresh"
- per-terminal checkboxes
- badges: `reattach`, `reconnect`, `recreate`, `review`
- transcript preview
- last command preview
- safe action buttons:
  - `Open shell`
  - `Reconnect SSH`
  - `Reattach tmux`
  - `Copy last command`
  - `Run again` only after explicit click

Do not automatically re-run commands.

### 5. Last Command Handling

Use existing command history capture where available.

For shells without reliable history capture:

- PowerShell prompt hook emits OSC-7337
- Bash/Zsh prompt hooks emit OSC-7337
- SSH/tmux paths should preserve hook behavior where possible

For commands that were likely still running when the session ended, mark them:

```json
{
  "command": "kubectl logs -f deployment/api",
  "status": "unknown_after_shutdown",
  "canRunAgain": true,
  "requiresConfirmation": true
}
```

## Safety Rules

Never auto-run commands after reboot.

Dangerous command patterns should get extra friction:

- `rm`
- `del`
- `Remove-Item`
- `kubectl delete`
- `docker rm`
- `docker compose down`
- `systemctl restart`
- package install/update commands
- scripts with unknown content

For these, show:

- command
- previous CWD / host
- timestamp
- risk warning
- copy-only by default

## Relationship To tmux

tmux remains valuable for runtime persistence:

- Mimir closes -> tmux keeps shell alive
- SSH drops -> remote tmux keeps shell alive
- GUI crashes -> tmux keeps shell alive

tmux does not survive reboot. After reboot, Mimir should restore the workspace
and attempt to recreate or reconnect where possible.

## Relationship To Windows ConPTY Agent

A future Windows ConPTY session agent would solve runtime persistence for native
Windows shells:

- Mimir closes -> PowerShell/cmd keeps running
- Mimir reopens -> attach to existing ConPTY session

It still would not preserve processes through reboot. It should integrate with
Reboot Workspace Restore by exposing:

- live sessions
- detached sessions
- transcript buffers
- last command metadata
- session IDs

## Benefits

- Strong admin experience after patch/reboot cycles
- Less cognitive load during incidents
- Better parity between Linux/tmux and Windows admins
- Makes history/transcript/session features feel connected
- Does not require unsafe background command re-execution
- Works even without Kubernetes/Docker/SSH test environments

## Costs / Tradeoffs

- More state management
- More restore edge cases
- Needs careful UX so users understand what is live vs recreated
- Risk of stale/incorrect context if timestamps and status are unclear
- Needs strict command re-run safety
- More platform-specific behavior

## Risks

### False Sense Of Continuity

Users may think a process survived reboot when it was recreated.

Mitigation:

- clear badges: `reattached`, `recreated`, `transcript only`
- show reboot/session boundary timestamp
- show "previous output" styling for restored transcripts

### Accidental Dangerous Re-run

Admins may click run again on a dangerous command.

Mitigation:

- never auto-run
- classify obvious dangerous commands
- require confirmation
- copy-only default for high-risk commands

### Snapshot Corruption

Crash during write could corrupt restore state.

Mitigation:

- atomic writes
- schema version
- keep last N snapshots
- validate before restore

### Sensitive Transcript Data

Restored transcripts may contain secrets.

Mitigation:

- reuse recording/transcript scrubbing where possible
- allow per-terminal "exclude from restore transcript"
- global setting for transcript retention

## Implementation Phases

### Phase 1 - Snapshot Foundation

- Define `workspace` package
- Snapshot schema
- Atomic save/load
- Keep last N snapshots
- Unit tests for validation and migration

### Phase 2 - Existing Session State Integration

- Extend current session persistence with:
  - CWD
  - last command
  - last exit code
  - restore reason/status
  - transcript excerpt path
- Preserve layout tree and terminal names
- Add restore planner

### Phase 3 - Restore UI

- Startup restore screen
- Per-terminal restore actions
- Transcript preview
- Last-command copy/run-again
- Clear live/recreated/transcript-only badges

### Phase 4 - tmux-aware Reconnect

- Detect known tmux session names
- Attempt local/SSH tmux reattach
- Mark missing tmux sessions as `process_gone`
- Do not silently create same-named tmux session without telling the user

### Phase 5 - Windows Native Runtime Persistence

- Optional per-user ConPTY session agent
- Named Pipe IPC
- Attach/detach native PowerShell/cmd sessions
- Integrate agent sessions into restore planner

## MVP Acceptance Criteria

- After app restart, previous terminal layout is restored.
- After machine reboot, previous workspace is offered for restore.
- Restored terminals clearly indicate whether they are live, recreated, or transcript-only.
- Last transcript is visible for each restored terminal.
- Last command is visible and copyable.
- "Run again" requires explicit user action.
- SSH/tmux sessions attempt reattach when configured.
- Native PowerShell/cmd are recreated with previous metadata, not presented as still live.

## Open Questions

- How many snapshots should be retained?
- Should restore be automatic or always user-confirmed after reboot?
- How do we detect a reboot reliably and cross-platform?
- Should transcript restore be opt-in for security?
- Should "run again" exist for high-risk commands or be copy-only?
- How much workflow/playbook state should be restored?
- Should restore state be portable/exportable for incident handoff?
