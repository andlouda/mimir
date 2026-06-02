# Workflow-Engine Plan

## Ziel

Mimir soll eine modulare Workflow-Engine erhalten, die:

- Tools und Templates als kleine, wiederverwendbare Bausteine nutzt
- AI-Schritte und menschliche Eingriffe kombinieren kann
- sichere und nachvollziehbare Ausfuehrung ermoeglicht
- spaeter fuer K8s, Docker und Projekt-/Claude-Kontext ausgebaut werden kann

## Design-Prinzipien

- Kleine Pakete mit klarer Verantwortung
- Keine freie Shell-Autonomie fuer AI
- Zentrale Risk-/Approval-Policy
- Explizite, serialisierbare Workflow-Definitionen
- UI-Zustand getrennt von Workflow-Engine
- Test-first fuer Engine-, Tool- und Policy-Kernlogik

## Zielstruktur

```text
mimir/
  workflow/
    types.go
    engine.go
    registry.go
    approval.go
    state.go
  tools/
    types.go
    registry.go
    template_tool.go
  context/
    terminal.go
    templates.go
    project_files.go
  frontend/src/lib/workflows/
    WorkflowCatalog.svelte
    WorkflowRunner.svelte
    WorkflowStepView.svelte
    WorkflowApproval.svelte
    WorkflowLog.svelte
```

## Phasen

### Phase 1: Backend-Grundgeruest

Ziel:
- Workflow-Typen und Tool-Schnittstellen einfuehren
- bestehende Template-Tools adaptieren

Aufgaben:
- `tools/types.go` mit `Tool`, `Parameter`, `ToolResult`, `RiskLevel`
- `tools/template_tool.go` als Adapter fuer `template.Template`
- `tools/registry.go` fuer Tool-Lookup
- `workflow/types.go` fuer `Workflow`, `WorkflowStep`, `WorkflowMode`
- `workflow/state.go` fuer `WorkflowState`, `WorkflowEvent`

Akzeptanzkriterien:
- Template-Tools koennen ueber ein einheitliches Tool-Interface gelesen werden
- Unit-Tests fuer Registry und Tool-Adapter vorhanden

### Phase 2: Workflow-Engine

Ziel:
- typisierte Schritte ausfuehren koennen

Aufgaben:
- `workflow/engine.go`
- `StepExecutor`-Schnittstelle
- `RunToolExecutor`
- `AskAIExecutor`
- `AskUserExecutor`
- serialisierbares Step-/Event-Logging

Akzeptanzkriterien:
- ein einfacher Workflow mit 2-3 Schritten ist backendseitig ausfuehrbar
- Engine-Tests decken Step-Sequenz und Fehlerpfade ab

### Phase 3: Safety / Approval

Ziel:
- riskante Schritte zentral absichern

Aufgaben:
- `workflow/approval.go`
- Mapping `RiskLevel -> ApprovalRequirement`
- Policy fuer `low`, `medium`, `high`
- Workflow-Stop auf Approval-Wartezustand

Akzeptanzkriterien:
- `medium` und `high` werden nicht automatisch ausgefuehrt
- Approval-Logik ist separat testbar

### Phase 4: Frontend-Runner

Ziel:
- Workflows im UI sichtbar und schrittweise steuerbar machen

Aufgaben:
- `WorkflowCatalog.svelte`
- `WorkflowRunner.svelte`
- `WorkflowStepView.svelte`
- `WorkflowApproval.svelte`
- `WorkflowLog.svelte`
- Wails-Bindings fuer Workflow-Start, Step-Fortschritt, Approval, Resume

Akzeptanzkriterien:
- ein Workflow kann gestartet, angezeigt und fortgesetzt werden
- Approval erscheint als klarer UI-Schritt statt stiller Blockade

### Phase 5: Erste produktive Workflows

Ziel:
- reale Nutzbarkeit mit kleinen, sicheren Workflows

Kandidaten:
- `Docker Compose Debug`
- `K8s Incident Triage`
- `Project Context Bootstrap`

Akzeptanzkriterien:
- mindestens ein read-only Workflow laeuft stabil
- mindestens ein Workflow enthaelt AI + User-Schritt

### Phase 6: Context Provider

Ziel:
- bessere AI- und Workflow-Entscheidungen durch strukturierten Kontext

Aufgaben:
- `TerminalContextProvider`
- `TemplateCatalogProvider`
- `ProjectFilesProvider`
- spaeter `DockerContextProvider`, `K8sContextProvider`

Akzeptanzkriterien:
- Planner/Explainer muessen Kontext nicht ad hoc in `app.go` zusammensetzen

## Erste konkrete Milestones

### Milestone A

- Tool-Interface
- Template-Tool-Adapter
- Tool-Registry
- Tests

### Milestone B

- Workflow-Typen
- Engine mit `run_tool`
- erster statischer Beispiel-Workflow

### Milestone C

- `ask_ai` und `ask_user`
- Approval-Policy
- UI-Runner fuer einen Workflow

## Testing-Strategie

- Reine Engine-Tests ohne Terminal-Prozesse
- Mock-Tools statt echter Shell-Ausfuehrung
- Approval-Policy als table-driven Tests
- Frontend-Komponenten moeglichst isoliert testen, sobald Test-Setup vorhanden ist

## Offene Fragen

- Sollen Workflows zunaechst als JSON-Dateien oder im Go-Code registriert werden?
- Wie fein granular soll das Event-Log sein?
- Sollen AI-Planer spaeter Workflows dynamisch anpassen duerfen oder nur statische Schritte parametrieren?
- Wo wird Workflow-Laufzeitstatus persistiert, falls Resume spaeter benoetigt wird?

## Empfohlene Reihenfolge

1. Backend-Tooling und Workflow-Typen
2. Engine fuer `run_tool`
3. Approval-Policy
4. Frontend-Runner
5. erster echter Workflow
6. Context Provider

## Nicht Teil der ersten Iteration

- freier autonomer Agent mit Shell-Zugriff
- selbstmodifizierende Workflows
- verteilte/remote Workflow-Ausfuehrung
- Workflow-Scripting mit eigener DSL
