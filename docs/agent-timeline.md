# Agent-Zeitleiste

Mimir records what a coding agent in a pane did and what happened around it,
as one JSONL file per agent session. The tab "Zeitleiste" in the agent panel
shows the same data; the files are meant to be read by other tools (for
example a Python investigation script).

Everything here is **off by default**. The master switch is in Settings,
card "Agent-Zeitleiste". While it is off, no hook entries beyond the usual
ones are installed and no files are written.

## Files

| What | Where |
|---|---|
| Events | `~/.config/mimir/agent-timeline/<session_id>.jsonl` (Linux: `$XDG_CONFIG_HOME`; Windows: `%APPDATA%\mimir\agent-timeline\`) |
| Rotated file | `<session_id>.jsonl.1` once the file exceeds the size limit |
| Settings | `~/.config/mimir/telemetry_settings.json` |

Directory mode 0700, files 0600. Retention (default 14 days) and the size
limit (default 50 MB per session) are settings; a purge runs at start-up.

`session_id` is Claude Code's session UUID, the same value as the
transcript file name under `~/.claude/projects/...`, the `session_id` in
hook payloads and `session.id` in the agent's OpenTelemetry export. Only ids
matching `^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$` are accepted.

## Line format

One JSON object per line, fields in `snake_case`:

| Field | Meaning |
|---|---|
| `v` | schema version, currently `1` |
| `seq` | sequence number per session, monotonic, continues across restarts |
| `ts` | RFC 3339 with nanoseconds, UTC |
| `ts_kind` | empty = the source's own clock; `file_mtime` = time the hook file was written; `observed` = time of the sample that saw a process or connection |
| `source` | `hook`, `otel`, `transcript`, `proc`, `net`, `mimir` |
| `type` | see below |
| `session_id` | the agent session |
| `terminal_id` | Mimir pane at ingest time, absent when unknown |
| `prompt_id` | OpenTelemetry `prompt.id` (links everything of one user prompt) |
| `tool_use_id` | the tool call this event belongs to (hooks, telemetry, attributed processes and connections) |
| `tool_name` | `Bash`, `Read`, `Edit`, `Task`, `mcp__...` |
| `pid`, `ppid` | process ids; for hook events `pid` is the agent process |
| `attrs` | type-specific attributes, redacted (see below) |
| `confidence` | for `proc`/`net`: `strong`, `medium`, `weak`, `none` (how surely the event belongs to `tool_use_id`) |
| `redacted` | `true` when a secret or an oversize value was replaced |
| `anomaly` | `unattributed_process`, `outlived_tool`, `unattributed_connection`, `tool_failed` |

### Types

| source | type | attrs |
|---|---|---|
| hook | `session_start` | `cwd`, `start_source` (startup, resume, clear, compact), `transcript_path` |
| hook | `prompt_submit` | (no prompt text) |
| hook | `tool_pre` | `tool_input` (redacted, capped), `cwd` |
| hook | `tool_post` | `tool_input`, `response` = `{bytes, is_error, interrupted}` |
| hook | `stop`, `subagent_stop` | `stop_hook_active`, `agent_id`, `subagent: true` |
| hook | `notification` | `notification_type`, `message` |
| otel | `tool_result`, `tool_decision`, `api_request`, `api_error`, ... | the event's attributes without `prompt` and `user.*` (next step) |
| proc | `process_start`, `process_end` | `cmdline`, `exe`, `started_at`, `lifetime_ms` (later step) |
| net | `conn_open`, `conn_close` | `proto`, `local`, `remote`, `state` (later step) |
| mimir | `pane_bound`, `overflow` | `terminal_id`, `reason`, `dropped` |

### Redaction

Hook payloads are compacted **inside the hook process** before anything is
written: the tool response shrinks to its size and error flags, prompt text,
file contents and edit strings are dropped, strings are capped at 512 bytes
(the shell command at 4 KiB), and keys that name a secret (`api_key`,
`authorization`, `password`, `token`, ...) lose their value. Free text is
scanned for token shapes (Bearer, `sk-`, `AKIA`, `ghp_`, JWT, PEM blocks,
`KEY=value`). Replaced values read `[REDACTED]`; `redacted: true` marks the
event.

## Correlation keys

- **Pane ↔ session**: the hook runs as a child of the agent, so the hook
  file name carries the agent's pid; Mimir matches that against the pid it
  found in the pane. From then on the pane follows that session.
- **Tool call across sources**: `tool_use_id` is identical in the hook
  payload, the transcript's `tool_use` block and the agent's telemetry.
- **Process ↔ tool call** (later step): a process below the agent whose
  start time falls into the window between `tool_pre` and `tool_post` of
  the only running Bash call is attributed with confidence `strong`; with
  several calls in flight, a command line match gives `medium`, otherwise
  `weak`; outside every window `none` plus `anomaly: unattributed_process`.

## Reading the files

```sh
jq -r 'select(.type=="tool_pre") | [.ts, .tool_name, .attrs.tool_input.command // .attrs.tool_input.file_path] | @tsv' \
  ~/.config/mimir/agent-timeline/<session_id>.jsonl
```

```python
import json, pathlib
for line in pathlib.Path("~/.config/mimir/agent-timeline/<session_id>.jsonl").expanduser().open():
    ev = json.loads(line)
    if ev["type"] in ("tool_pre", "tool_post"):
        print(ev["ts"], ev["type"], ev.get("tool_name"), ev.get("tool_use_id"))
```

Run such scripts with `python -I` when the directory is not yours.
