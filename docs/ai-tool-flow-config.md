# AI Tool Flow Config

`RunAITemplateTool()` ist nicht mehr hart im Code verdrahtet. Der Ablauf kann jetzt über `yaml` oder `json` konfiguriert werden.

Wichtig:

- Die Config ist **nicht** die letzte Sicherheitsgrenze.
- Einige Kernregeln werden immer durch [`aiflow.ApplyImmutableGuardrails()`](aiflow/guardrails.go:1) erzwungen.
- Details dazu stehen in [`docs/ai-deterministic-guardrails.md`](docs/ai-deterministic-guardrails.md:1).

## Suchpfade

In dieser Reihenfolge:

1. `MIMIR_AI_TOOL_FLOW_CONFIG`
2. User-Config-Datei: `ai_tool_flow.json` im Mimir-Konfigurationsverzeichnis
3. `config/ai_tool_flow.yaml`
4. `config/ai_tool_flow.yml`
5. `config/ai_tool_flow.json`

Wenn keine Datei gefunden wird, nutzt Mimir sichere Defaults aus `aiflow.DefaultConfig()`.

## Runtime-Änderung

Der Pre-Prompt kann zur Laufzeit über die App-UI geändert werden. Beim Speichern schreibt Mimir standardmäßig in die User-Config-Datei.

Wenn `MIMIR_AI_TOOL_FLOW_CONFIG` gesetzt ist, liest und schreibt Mimir stattdessen diese Datei.

## Bereiche

### `prompt`

- `requireStableToolId`: AI soll `toolId` statt freier Namen zurückgeben
- `prePrompt`: frei editierbarer Pre-Prompt/System-Text für die Tool-Auswahl
- `includeRisk`: Risk-Level in die Toolbeschreibung für die AI aufnehmen
- `includeCategory`: Kategorie in die Toolbeschreibung aufnehmen
- `includeTerminalOutput`: aktuellen Terminal-Output in den Prompt aufnehmen
- `maxTerminalContext`: maximale Prompt-Länge für Terminal-Output
- `allowTemplateNameFallback`: Legacy-Fallback auf Template-Namen erlauben

### `toolFilter`

- `includeCategories`
- `excludeCategories`
- `includeToolIds`
- `excludeToolIds`

Damit kann der AI-Toolkatalog pro Deployment oder Umgebung eingeschränkt werden, ohne Templates selbst umzubauen.

### `approval`

- `respectStepFlag`
- `requireApprovalForLow`
- `requireApprovalForMedium`
- `requireApprovalForHigh`

Damit kann dieselbe Workflow-/Approval-Architektur unterschiedlich streng gefahren werden.

### `execution`

- `enabled`
- `workflowMode`
- `workflowIdPrefix`
- `workflowName`
- `forceRequiresApproval`

Damit ist der AI-initiierte Tool-Run als normaler, konfigurierbarer Workflow-Schritt modelliert.

### `providerPolicies`

Provider-spezifische Guardrails für Tool-Auswahl und Kontextumfang.

Felder:

- `allowedToolClasses`
- `allowSensitiveContext`
- `maxContextChars`

Beispiel:

- `openai`
  - nur `safe_readonly`
  - strenger sanitiserter Kontext
- `ollama`
  - `safe_readonly` plus `sensitive_readonly`
  - größerer Kontextumfang

Damit kann lokaler Ollama-Betrieb kontrolliert offener sein als Cloud-Provider-Nutzung.

## Beispiel

Siehe [`config/ai_tool_flow.yaml`](config/ai_tool_flow.yaml:1).
