# ADR-0009: Modulare Workflow-Engine mit Tool-, Approval- und AI-Trennung

## Status

Vorgeschlagen

## Kontext

Mimir hat aktuell mehrere produktive Bausteine, die voneinander getrennt gewachsen sind:

- Template-Ausfuehrung ueber `template.Manager`
- AI-Aktionen wie `Explain Output`, `Suggest Next Command` und `Run Template Tool From Goal`
- einen Function Catalog fuer AI-Aktionen und Template-Tools
- Terminal-, Datei- und Template-Kontext, der heute nur punktuell verwendet wird

Die naechste Ausbaustufe soll Workflows ermoeglichen, die:

- aus mehreren kleinen Schritten bestehen
- sowohl menschliche als auch AI-getriebene Schritte enthalten
- sicherheitskritische Aktionen nur nach ausdruecklicher Zustimmung ausfuehren
- spaeter fuer K8s, Docker, Claude-/Projektkontext und weitere Tool-Arten erweiterbar bleiben

Ein monolithischer "Agent" waere fuer diesen Stand zu unkontrolliert. Er wuerde:

- Shell-Ausfuehrung, Planung, Safety und UI-Zustand vermischen
- schwer testbar sein
- das Risiko von unbeabsichtigten Aktionen erhoehen
- die bestehende Codebasis weiter in `App.svelte` und `app.go` konzentrieren

## Entscheidung

Es wird eine **modulare Workflow-Architektur** eingefuehrt, die folgende Ebenen strikt trennt:

1. **Tools**
2. **Workflow-Definitionen**
3. **Workflow-Engine / Step-Executors**
4. **Approval- / Safety-Policy**
5. **Context Provider**
6. **AI-Planung und AI-Erklaerung**

Workflows werden nicht als freie Shell-Skripte modelliert, sondern als Folge typisierter Schritte. AI darf innerhalb dieser Struktur planen, erklaeren und Parameter vorbereiten, aber nicht unkontrolliert beliebige Shell-Befehle erzeugen und ausfuehren.

### 1. Tool-Abstraktion

Jede ausfuehrbare Funktion bekommt eine explizite Tool-Schnittstelle:

```go
type Tool interface {
    Name() string
    Description() string
    Category() string
    Risk() RiskLevel
    Parameters() []Parameter
    Run(ctx RunContext, input map[string]string) (ToolResult, error)
}
```

Erste Tool-Klassen:

- `TemplateTool` fuer bestehende Template-Tools
- spaeter `TerminalTool`, `FileTool`, `ContextTool`

### 2. Workflow-Definition

Workflows werden als kleine, serialisierbare Definitionen abgelegt:

```go
type Workflow struct {
    ID          string
    Name        string
    Description string
    Mode        WorkflowMode
    Steps       []WorkflowStep
}

type WorkflowStep struct {
    ID               string
    Type             StepType
    Tool             string
    Prompt           string
    Inputs           map[string]string
    RequiresApproval bool
}
```

Erste Step-Typen:

- `run_tool`
- `ask_ai`
- `ask_user`

### 3. Workflow-Engine

Die Engine fuehrt Workflows nicht ueber eine zentrale `switch`-Datei aus, sondern ueber kleine Step-Executors:

```go
type StepExecutor interface {
    CanHandle(step WorkflowStep) bool
    Execute(state *WorkflowState, step WorkflowStep) error
}
```

Erste Executor:

- `RunToolExecutor`
- `AskAIExecutor`
- `AskUserExecutor`

### 4. Safety / Approval

Riskante Schritte werden zentral bewertet und nicht pro Tool ad hoc behandelt.

```go
type RiskLevel string

const (
    RiskLow    RiskLevel = "low"
    RiskMedium RiskLevel = "medium"
    RiskHigh   RiskLevel = "high"
)
```

Regeln:

- `low`: darf im Assist-/Auto-Modus ohne extra Bestaetigung laufen
- `medium`: braucht Bestaetigung oder explizite Workflow-Freigabe
- `high`: nie stillschweigend automatisch ausfuehren

Die Approval-Entscheidung sitzt in einem separaten Modul und nicht in Tool- oder AI-Code.

### 5. Context Provider

Kontext wird ueber kleine Provider gesammelt:

```go
type ContextProvider interface {
    Name() string
    Collect() (map[string]any, error)
}
```

Erste Kandidaten:

- `TerminalContextProvider`
- `TemplateCatalogProvider`
- `ProjectFilesProvider`
- spaeter `DockerContextProvider`, `K8sContextProvider`

### 6. AI-Rollen

AI wird nicht als allgemeiner Vollagent eingefuehrt, sondern in getrennten Rollen:

- **Explainer**: erklaert Tools, Outputs und Workflows
- **Planner**: waehlt Tools oder naechste Schritte innerhalb klarer Grenzen

AI-Aufrufe bleiben backendseitig, werden wie bisher geloggt und koennen spaeter provider-unabhaengig bleiben.

## Konsequenzen

### Positiv

- **Lesbarkeit**: kleine, fokussierte Dateien statt weiterer Logik in `app.go` und `App.svelte`
- **Erweiterbarkeit**: neue Tool-Arten und Workflow-Schritte koennen additiv hinzukommen
- **Testbarkeit**: Tool-, Policy- und Engine-Logik sind getrennt unit-testbar
- **Sicherheit**: Risk-/Approval-Logik wird zentral statt implizit
- **Produktfit**: menschliche Interaktion und AI-Unterstuetzung koennen in denselben Workflows koexistieren

### Negativ

- **Mehr Strukturaufwand**: zusaetzliche Pakete und Datentypen vor dem ersten sichtbaren Workflow
- **Hoeherer Orchestrierungsbedarf**: State- und UI-Flows muessen sauber modelliert werden
- **Anfangs doppelte Konzepte**: Templates, Tools und Function Catalog existieren parallel, bis die Integration vollstaendig ist

## Alternativen

| Alternative | Grund fuer Ablehnung |
|---|---|
| Freier AI-Agent mit direkter Shell-Ausfuehrung | Zu riskant, schwer testbar, schwer nachvollziehbar |
| Workflows direkt als Shell-Skripte | Schlechte Parametrisierung, keine saubere Approval-/UI-Integration |
| Alles in `app.go` / `App.svelte` belassen | Wuerde aktuelle Monolith-Tendenzen verstaerken |
| Nur Templates ausbauen, keine Workflows | Reicht fuer mehrstufige Mensch+AI-Ablaufe nicht aus |

## Betroffene Dateien / Module

Geplante neue Backend-Module:

- `workflow/types.go`
- `workflow/engine.go`
- `workflow/registry.go`
- `workflow/approval.go`
- `tools/types.go`
- `tools/registry.go`
- `tools/template_tool.go`
- spaeter `context/*.go`

Geplante neue Frontend-Module:

- `frontend/src/lib/workflows/WorkflowCatalog.svelte`
- `frontend/src/lib/workflows/WorkflowRunner.svelte`
- `frontend/src/lib/workflows/WorkflowStepView.svelte`
- `frontend/src/lib/workflows/WorkflowApproval.svelte`
- `frontend/src/lib/workflows/WorkflowLog.svelte`

## Migrationsnotizen

- Bestehende Template-Tools bleiben erhalten und werden zuerst nur adaptiert, nicht ersetzt.
- Der vorhandene Function Catalog bleibt bestehen und wird spaeter aus derselben Tool-/Workflow-Registry gespeist.
- Die ersten Workflows werden read-only oder low-risk gehalten, bevor mutierende Aktionen freigegeben werden.
