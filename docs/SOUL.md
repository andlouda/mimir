# Mimir

*Named after the Norse god of wisdom — the being who guarded the well of knowledge beneath Yggdrasil, the world tree. Odin gave his eye for a single drink from that well. Mimir asks for nothing. It just opens the well.*

---

## What Mimir Is

Mimir is a terminal multiplexer that believes infrastructure work shouldn't feel like archaeology. Every server you SSH into, every container you inspect, every log you tail — it's a conversation with a machine. Mimir is the desk where those conversations happen.

It's a single binary. No Electron. No browser tab pretending to be an app. Go on the backend, Svelte on the frontend, Wails holding them together. It starts in under a second and sits in your system tray like it belongs there.

## The Feeling

The feeling Mimir aims for is **quiet competence**. Like a well-worn leather tool roll — everything in its place, nothing you don't need. The interface is dark (`#0c0e14`), the accent is a calm blue (`#63b3ed`), and the typography is monospace. There are no onboarding wizards, no "getting started" banners, no confetti animations. You open it, and there's a terminal. You know what to do.

When you split a pane, the new terminal connects to the same host. When you apply a template, variables auto-populate from discovery. When you record a session, the output is an Asciicast you can replay or export as GIF. Everything works the way a systems person expects — because Mimir was built by one.

## The Problem It Solves

Infrastructure people live in terminals. They SSH into production, run five commands they half-remember, scroll through logs hoping something jumps out, switch to another tab to check Kubernetes, come back and forget what they were doing.

The tools they have are either too simple (a plain terminal) or too complex (full observability platforms with dashboards, alert rules, and a three-month onboarding). There's a gap between "I have a shell" and "I have a $50k/year SaaS platform." Mimir lives in that gap.

It's not a monitoring tool. It's not an observability platform. It's the place where the person who *uses* those things does their actual work.

## Architecture

Mimir is built around a small set of ideas that reinforce each other:

### Terminals Are First-Class

Every terminal is a `TerminalSession` — an interface with four methods: `Read`, `Write`, `Resize`, `Close`. Local shells, SSH connections, and Windows ConPTY all implement it identically. The frontend doesn't know or care what's on the other end. A bash prompt and an SSH session to a Kubernetes node look the same, split the same, record the same.

The layout is a binary tree. Every split creates two children. Every leaf is a terminal. You can nest splits arbitrarily. The tree persists across sessions — close Mimir, reopen it, and your layout is waiting. SSH sessions reconnect. Tmux sessions reattach. Local shells restore their transcript.

### Templates Are Commands With Memory

A template is a parameterized command: `docker compose logs -f {{.Service}}`. Templates are categorized (Containers, Kubernetes, Network, System), can have discovery tools attached to auto-populate variables, and work across terminal types with fallback chains (SSH falls back to bash).

They're JSON files, embedded in the binary but editable by the user. 58 ship out of the box. They cover the commands that infrastructure people run ten times a day but never bother to alias — because the parameters change every time.

### Workflows Are Templates That Think

A workflow is a sequence of steps: run a discovery, execute a tool, ask AI to interpret the output, pause for human approval, continue. They bridge the gap between "I know what commands to run" and "I want the computer to run them in order and tell me what it finds."

Four built-in playbooks ship with Mimir: Docker Debug, K8s Cluster Overview, Host Basic Triage, API / Network Health Check. Each one is a distillation of what an experienced engineer does in the first ten minutes of an incident — except it doesn't forget steps and it doesn't panic.

Workflows have three modes:
- **Assist** — run everything, present findings
- **Approve** — pause before each step, let the human decide
- **Auto** — full autonomy (for the brave)

Discovery steps are non-fatal. If Docker Compose isn't installed, the workflow continues with empty results instead of crashing. Infrastructure is messy. The tool should handle that.

### AI Is a Copilot, Not a Pilot

Mimir integrates OpenAI and Ollama, but AI is never in control. It reads terminal output and suggests — it doesn't execute. Every AI interaction goes through guardrails that block destructive commands (`rm -rf`, `kubectl delete`, `docker rm`, `shutdown`). The prompt is explicit: *"You are a cautious infrastructure troubleshooting assistant. Prefer read-only reasoning, highlight uncertainty."*

AI modes are built-in shortcuts: "explain this output," "suggest the next command," "summarize findings." They're available from any terminal via the AI panel. The interaction log records every prompt, response, and redaction — because in infrastructure, you need to know what the computer told you and why.

### Security Is Structural

SSH passwords go through the system keyring (or encrypted storage where keyrings aren't available). Host key verification follows TOFU (Trust On First Use) — the first connection shows the fingerprint in a modal, subsequent connections verify against the stored key. `InsecureIgnoreHostKey` is never called.

Secrets are never logged. Terminal output sent to AI goes through a scrubber that redacts patterns matching API keys, tokens, and passwords. The redaction count is recorded in the interaction log.

### Everything Is a File

Configuration lives in `~/.config/mimir/`. Session state, AI settings, SSH profiles, playbooks, recordings, command history, notes — all JSON files in known locations. No database. No migration scripts. You can back up Mimir by copying a directory. You can reset it by deleting one.

## What Mimir Is Not

Mimir is not a cloud service. It doesn't phone home, it doesn't require an account, it doesn't collect telemetry. It's a desktop application that runs on your machine and talks to your servers.

It's not a replacement for proper monitoring. It doesn't have alerting, it doesn't draw time-series graphs, it doesn't aggregate metrics across fleets. It's the tool you reach for when the alert fires and you need to figure out what's actually happening.

It's not an IDE. It doesn't edit code, it doesn't lint, it doesn't auto-complete. It's a terminal multiplexer with opinions about how infrastructure troubleshooting should feel.

## The Name

In Norse mythology, Mimir was the wisest of the Aesir. After the war between the Aesir and the Vanir, Mimir was beheaded. Odin carried the head with him, preserved it with herbs and magic, and consulted it for counsel. The head spoke truths that others couldn't see.

A terminal multiplexer named Mimir is a severed head that speaks truth about your infrastructure. You carry it with you. You ask it questions. It tells you what it sees. Sometimes the answer is uncomfortable. That's the point.

---

*~29,000 lines of code. 89 Go files. 29 Svelte components. One binary. No dependencies at runtime. Ships cross-platform: Linux, macOS, Windows.*
