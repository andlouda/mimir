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
| otel | `tool_result`, `tool_decision`, `api_request`, `api_error`, `api_refusal`, `user_prompt`, `permission_mode_changed`, `mcp_connection`, `skill_activated`, `otel_other` | whitelisted attributes per type (`success`, `duration_ms`, `decision`, `model`, `cost_usd`, token counts, `request_id`, ...); never `prompt`, `user.*`, `organization.id`; `tool_parameters` only when the tool-details setting is on |
| otel (traces beta) | `span_interaction`, `span_llm`, `span_tool`, `span_tool_exec`, `span_tool_blocked`, `span_hook` | `trace_id`, `span_id`, `parent_span_id`, `duration_ms`, `status`, tool/model attributes |
| proc | `process_start`, `process_end` | `cmdline` (redacted), `started_at`, `lifetime_ms` |
| net | `conn_open`, `conn_close` | `proto` (tcp, tcp6, udp, udp6), `local`, `remote`, `state`, `inode` |
| mimir | `observe_started` | `procs`, `conns` at the baseline snapshot |
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

## Telemetry receiver

With "Telemetrie des Agenten empfangen" on, Mimir listens on
`http://127.0.0.1:<port>` (default 14318) for OTLP/HTTP in JSON encoding
(`/v1/logs`, `/v1/traces`, `/v1/metrics`; metrics are acknowledged and
dropped). It binds loopback only, refuses other content types, caps bodies
at 4 MiB and, by default, requires the shared token in the `x-mimir-token`
header (`Authorization: Bearer <token>` works too). The token lives in
`telemetry_settings.json` (0600) and is only handed out inside the copyable
environment block.

Claude Code needs these variables (Settings → "Für bash/zsh kopieren"):

```sh
export CLAUDE_CODE_ENABLE_TELEMETRY=1
export OTEL_LOGS_EXPORTER=otlp
export OTEL_METRICS_EXPORTER=none
export OTEL_EXPORTER_OTLP_PROTOCOL=http/json
export OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:14318
export OTEL_LOGS_EXPORT_INTERVAL=1000
export OTEL_EXPORTER_OTLP_HEADERS=x-mimir-token=<token>
```

Mimir's own local shells get these variables automatically while the
receiver is on: bash and zsh through the rc file Mimir already writes per
launch (so they also reach shells inside an existing `tmux -L mimir`
server), PowerShell and cmd through the process environment, WSL through
the rc file inside the distro. A running shell keeps its old environment
until the pane is restarted. SSH panes are not covered: the remote host's
`127.0.0.1` is not Mimir's, and nothing is installed on SSH servers; the
hook one-liner still delivers tool events there. Under WSL2, `127.0.0.1`
reaches the Windows listener only with mirrored networking.

`OTEL_LOG_USER_PROMPTS` is never set. Records whose `event.name` is not
`claude_code.*` are counted and dropped; records for a session no pane has
claimed are stored under their session id but not shown live (cap: 50 new
sessions per hour). Claude Code supports one endpoint per signal, so a user
with a real collector has to choose.

## Runtime observation

With "Prozesse und Verbindungen der Pane beobachten" on (Linux only for
now), Mimir samples the process tree below the agent through `/proc`:
`/proc/<pid>/task/<pid>/children` for the tree, `stat` for ppid and start
time, `cmdline`, the `fd` directory for socket inodes, and `/proc/net/tcp`
and `tcp6` (udp on request) once per sample to resolve them. No root, own
processes only; processes in another network namespace (containers) are
not visible. Processes are keyed by pid plus start time, so pid reuse is a
distinct process. Bounds: 300 processes, 2000 descriptors per sample; above
that an `overflow` event is written once. The interval is the setting
(default 1 s) while a tool call is in flight and three times that otherwise.
Everything that already ran when observation started is the baseline and
is not reported. Processes shorter than one interval are missed.

On Windows the sampler is native (Toolhelp32 for the tree, GetProcessTimes
for the start time, QueryFullProcessImageName for the image path,
GetExtendedTcpTable for TCP with owning pids): no PowerShell per tick. It
has no command lines (attribution is strong or weak, never medium) and no
UDP; the interval is at least 2 s. macOS has no sampler yet.

Subagents: with the traces setting on, the subagent's tool calls carry
spans whose parent span is the parent's Task call; the timeline tab nests
them under that call.

## Correlation keys

- **Pane ↔ session**: the hook runs as a child of the agent, so the hook
  file name carries the agent's pid; Mimir matches that against the pid it
  found in the pane. From then on the pane follows that session.
- **Tool call across sources**: `tool_use_id` is identical in the hook
  payload, the transcript's `tool_use` block and the agent's telemetry.
- **Process ↔ tool call**: a process below the agent whose start time
  falls into the window between `tool_pre` and `tool_post` (or the
  telemetry's `tool_result`) of the only running spawning tool (Bash, Task,
  Skill, MCP tools) is attributed with confidence `strong`; with several
  calls in flight, a command line match gives `medium`, otherwise `weak`
  (earliest window); outside every window `none` plus
  `anomaly: unattributed_process`. Connections inherit their process's
  attribution; one from an unattributed process gets
  `unattributed_connection`. A process still alive after its window closed
  gets `outlived_tool` on its `process_end`. Runtime events wait 3 s before
  they are resolved so the hook file, which the watcher polls every 2 s,
  can arrive first; windows are widened by 1 s on both sides.

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
