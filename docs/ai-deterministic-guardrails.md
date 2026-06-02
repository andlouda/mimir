# AI Deterministic Guardrails

Diese Datei beschreibt die **deterministischen** Guardrails für AI-Ausführung in Mimir.

Wichtig:

- Diese Guardrails sind **nicht** nur Prompt-Text.
- Sie werden **backendseitig** validiert und gefiltert.
- User-Settings, YAML/JSON-Konfiguration und editierbare Pre-Prompts können diese Kernregeln **nicht** abschalten.

## Ziel

AI darf in Mimir nicht:

- freie Shell-Kommandos erfinden
- mutierende oder destruktive Tools ausführen
- Secrets aus Terminal-Kontext oder Dateien weiterreichen
- kritische Parameter frei halluzinieren

Stattdessen gilt:

1. AI sieht nur einen gefilterten Tool-Katalog.
2. Der Terminal-Kontext wird vor jedem AI-Call sanitisiert.
3. AI muss genau ein registriertes Tool per stabiler `toolId` wählen.
4. Parameter werden gegen harte Regeln validiert.
5. Die Ausführung läuft nur über Workflow-/Approval-Codepfade.

## Enforcement-Reihenfolge

### 1. Immutable Config Hardening

Quelle:
- [`aiflow/guardrails.go`](aiflow/guardrails.go:1)

Vor jeder AI-Tool-Ausführung wird die geladene Konfiguration hart überschrieben:

- `RequireStableToolID = true`
- `AllowTemplateNameFallback = false`
- `RespectStepFlag = true`
- `RequireApprovalForMedium = true`
- `RequireApprovalForHigh = true`

Damit kann eine User-Konfiguration diese Sicherheitsbasis nicht lockern.

### 2. Tool Class Model

Quelle:
- [`tools/types.go`](tools/types.go:1)
- [`tools/template_tool.go`](tools/template_tool.go:1)
- [`template/template.go`](template/template.go:1)

Jedes Tool hat jetzt neben `risk` auch eine feste Klasse:

- `safe_readonly`
- `sensitive_readonly`
- `mutating`
- `destructive`
- `secret_access`

Diese Klasse wird bei Template-Tools aus Metadaten bzw. konservativen Defaults abgeleitet.

### 3. Provider-Based Allowlist

Quelle:
- [`aiflow/config.go`](aiflow/config.go:1)
- [`config/ai_tool_flow.yaml`](config/ai_tool_flow.yaml:1)

Die Tool-Allowlist hängt jetzt vom Provider ab:

- `openai`
  - standardmäßig nur `safe_readonly`
  - strikter Sanitizer
  - kleineres Kontextfenster
- `ollama`
  - standardmäßig `safe_readonly` und `sensitive_readonly`
  - lockerer Kontextumfang
  - aber weiterhin Secret-Redaction

Damit ist lokales Modell-Running kontrolliert offener als Cloud-Nutzung.

### 4. Terminal Context Isolation

Quelle:
- [`aiflow/context.go`](aiflow/context.go:1)
- [`ai.go`](ai.go:1)

Vor **jedem** AI-Call wird Terminal-Kontext deterministisch verarbeitet:

1. ANSI-Sequenzen entfernen
2. Zeilenenden normalisieren
3. Secrets redigieren
4. bei strikten Providern sensible Zeilen komplett entfernen
5. Kontext auf das erlaubte Zeichenlimit am Ende beschneiden

Das passiert nicht nur für `RunAITemplateTool()`, sondern auch für:

- `Explain Output`
- `Suggest Next Command`
- `Write Command From Goal`

### 5. Secret Redaction

Quelle:
- [`aiflow/context.go`](aiflow/context.go:1)

Vor dem Senden an ein Modell werden u. a. folgende Muster maskiert:

- OpenAI-API-Keys (`sk-...`)
- `Bearer ...`
- `Authorization: ...`
- Private-Key-Blöcke
- Session-/Cookie-Werte
- typische `.env`-Secrets
- kubeconfig-Tokens und Client-Key-Daten

Zusätzlich werden im strikten Provider-Modus sensible Pfade/Zeilen entfernt, z. B.:

- `.ssh/`
- `.gnupg/`
- `.aws/`
- `.kube/config`
- `.pem`
- `id_rsa`
- `id_ed25519`

### 6. AI Tool Catalog Filtering

Quelle:
- [`ai.go`](ai.go:365)
- [`aiflow/guardrails.go`](aiflow/guardrails.go:1)

Ein Template wird nur dann überhaupt AI-seitig registriert, wenn:

- `DangerLevel == "low"`
- keine blockierten Command-Fragmente enthalten sind

Danach wird der Katalog zusätzlich über:

- globale Tool-Filter
- Provider-Policy
- Tool-Klassen-Allowlist

reduziert.

### 7. Stable Tool Resolution

Quelle:
- [`ai.go`](ai.go:441)

AI muss eine registrierte `toolId` liefern.

Nicht erlaubt:

- freie Raw-Commands
- Template-Namen als Fallback
- unbekannte Tool-IDs

### 8. Deterministic Parameter Validation

Quelle:
- [`tools/types.go`](tools/types.go:1)
- [`aiflow/guardrails.go`](aiflow/guardrails.go:1)

Parameter können jetzt zusätzliche Regeln tragen:

- `type`
- `pattern`
- `maxLength`
- `options`
- `source`
- `discoveryTool`

Validierung vor Ausführung:

- nur deklarierte Parameter sind erlaubt
- `required` muss gesetzt sein
- `maxLength` wird erzwungen
- `options` wird erzwungen
- `pattern` wird erzwungen
- Shell-/Injection-Fragmente werden blockiert

Harte Wert-Blockliste u. a.:

- `;`
- `&&`
- `||`
- `|`
- `>`
- `<`
- `` ` ``
- `$(`
- `${`
- Newlines
- `'`
- `"`

### 9. Discovery-Before-Action

Quelle:
- [`tools/types.go`](tools/types.go:1)
- [`aiflow/guardrails.go`](aiflow/guardrails.go:1)
- Beispiel-Templates:
  - [`templates/k8s_logs_pod.json`](templates/k8s_logs_pod.json:1)
  - [`templates/docker_compose_logs.json`](templates/docker_compose_logs.json:1)
  - [`templates/docker_container_logs.json`](templates/docker_container_logs.json:1)
  - [`templates/k8s_describe_resource.json`](templates/k8s_describe_resource.json:1)

Parameterquellen:

- `ai_allowed`
- `user_only`
- `discovery_only`

Deterministische Regeln:

- `user_only` wird von AI-Ausführung blockiert
- `discovery_only` wird gegen echte Discovery-Ergebnisse validiert

Aktuell existiert dafür ein read-only Discovery-Resolver im Backend:

- [`aiflow/discovery.go`](aiflow/discovery.go:1)

Unterstützt sind derzeit:

- `discovery:list_k8s_namespaces`
- `discovery:list_k8s_pods`
- `discovery:list_docker_containers`
- `discovery:list_compose_services`
- `discovery:list_k8s_resources`

Eigenschaften:

- read-only Ausführung
- keine Shell-String-Konkatenation, sondern `exec.Command(...)` mit Argumenten
- Cache pro AI-Run
- Workdir-Unterstützung für Compose-Service-Discovery
- Security-Events bei Resolve/Failure/Denied

Damit kann AI kritische Zielnamen wie Pod-, Namespace- oder Container-Namen nicht frei halluzinieren.

### 10. Output Validation for Command Modes

Quelle:
- [`aiflow/guardrails.go`](aiflow/guardrails.go:1)
- [`ai.go`](ai.go:1)

Für `Suggest Next Command` und `Write Command From Goal` wird die AI-Antwort nach dem Provider-Call nochmal validiert.

Blockiert werden u. a.:

- leere Antwort
- Multi-Line-Ausgabe
- Markdown-Codefences
- mutierende/destruktive Command-Fragmente
- Command-Chains (`;`, `&&`, `||`)
- Pipes/Redirects
- `curl ... | sh`-artige Muster

Damit ist auch dieser Pfad nicht nur promptbasiert abgesichert.

### 11. Workflow / Approval Gate

Quelle:
- [`workflow/engine.go`](workflow/engine.go:1)

Nach erfolgreicher Tool- und Parameter-Validierung wird die Ausführung nur über die Workflow-Engine angestoßen.

`medium` und `high` bleiben approval-pflichtig, selbst wenn sie in der Config anders gesetzt würden.

## Immutable Deny Patterns

Quelle:
- [`aiflow/guardrails.go`](aiflow/guardrails.go:1)

Geblockte Kommandofragmente umfassen u. a.:

- Dateilöschung:
  - `rm -rf`
  - `rm -f`
  - `remove-item`
  - `del /s /q`
  - `del /f`
- Prozess-/Systemstop:
  - `kill`
  - `pkill`
  - `killall`
  - `taskkill`
  - `shutdown`
  - `reboot`
- Service-/Systemmutation:
  - `sc delete`
  - `sc stop`
  - `systemctl restart`
  - `systemctl stop`
  - `systemctl disable`
  - `service restart`
  - `service stop`
- Kubernetes-Mutation:
  - `kubectl delete`
  - `kubectl apply`
  - `kubectl patch`
  - `kubectl replace`
  - `kubectl scale`
  - `kubectl rollout restart`
- Docker-/Compose-Mutation:
  - `docker rm`
  - `docker stop`
  - `docker kill`
  - `docker restart`
  - `docker system prune`
  - `docker compose down`
  - `docker compose restart`
  - `docker compose rm`
  - `docker compose up`
- Paket-/Umgebungsänderungen:
  - `apt install`
  - `apt remove`
  - `apt purge`
  - `apt upgrade`
  - `dnf install`
  - `yum install`
  - `brew install`
  - `pip install`
  - `npm install`
- weitere Mutationen:
  - `truncate -s 0`
  - `mkfs`
  - `format `
  - `netsh advfirewall`
  - `ufw `
  - `iptables `
  - `reg add`
  - `reg delete`
  - `helm upgrade`
  - `helm uninstall`

## Was User konfigurieren dürfen

User dürfen weiterhin ändern:

- Provider
- Modell
- Endpoint
- editierbaren Intro-/Pre-Prompt
- globale Tool-Filter
- Teile von Approval/Execution
- Provider-Policies in Konfigurationsdateien

Aber:

- Immutable Hardening bleibt aktiv
- Secret-Redaction bleibt aktiv
- Tool-Class-/Risk-/Command-Checks bleiben aktiv
- Output-Validation bleibt aktiv

## Was User nicht abschalten können

Nicht abschaltbar sind aktuell:

- nur registrierte `toolId`
- kein Template-Name-Fallback
- Blockliste für mutierende/destruktive Kommandos
- Secret-Redaction vor AI-Calls
- Output-Validation für Command-Modi
- Parameter-Injection-Blockliste
- Blockade von `user_only`-Parametern
- Blockade von `discovery_only` ohne Discovery-Nachweis
- Approval für `medium` und `high`

## Testabdeckung

Tests:

- [`aiflow/guardrails_test.go`](aiflow/guardrails_test.go:1)
- [`aiflow/context_test.go`](aiflow/context_test.go:1)
- [`aiflow/config_test.go`](aiflow/config_test.go:1)

Abgedeckt sind u. a.:

- non-`low` risk blockiert
- destruktive Commands blockiert
- unbekannte Parameter blockiert
- `user_only` blockiert
- `discovery_only` blockiert
- suspicious Parameterwerte blockiert
- sichere read-only Auswahl erlaubt
- Provider-Allowlist (`openai` vs `ollama`)
- Secret-Redaction und Strict-Context-Filtering
- Output-Validation für Command-Modi

## Grenzen

Die Guardrails sind jetzt wesentlich stärker als reiner Prompt-Text, aber sie sind noch keine vollständige Sicherheitsplattform.

Aktuelle Grenzen:

- Discovery-Nachweis ist vorbereitet, aber noch nicht als echter Discovery-Cache / Resolver umgesetzt
- Blocklisten und Klassifikation sind konservativ und nicht semantisch perfekt
- Frontend kann die neuen Provider-Policies noch nicht vollständig editieren
- Rate-Limits / Cooldowns / Replay-Schutz sind noch nicht eingebaut

Wenn Mimir später kontrollierte mutierende AI-Aktionen zulassen soll, sollte das **nicht** durch Lockerung dieser Guardrails geschehen, sondern über:

- getrennte Tool-Klassen
- separate Human-in-the-loop-Flows
- explizite Discovery-Schritte
- signierte/prüfbare Execution-Pläne
