# Mimir Module & Marketplace — Architektur- und Umsetzungsplan

Status: Entwurf / Diskussionsgrundlage
Letzte Aktualisierung: 2026-06-05

Dieses Dokument beschreibt, wie Mimir ein **installierbares Modul-System** mit
einem **Marketplace** bekommen kann — vergleichbar mit Plugin-Stores anderer
Tools, aber zugeschnitten auf Mimirs Architektur (Wails, Single-Binary,
local-first) und sein Sicherheitsmodell.

Es ist als Entscheidungs- und Umsetzungsgrundlage gedacht: zuerst die
Rahmenbedingungen und die zentrale Designentscheidung, dann ein gestuftes
Konzept (Stufe 1–3), dann konkrete Datenstrukturen, Sicherheitsregeln und ein
phasenweiser Umsetzungsplan.

---

## 1. Ausgangslage

### 1.1 Was Mimir heute ist

- **Wails-Desktop-App**: Go-Backend + Svelte/xterm.js-Frontend, kompiliert zu
  *einer* ausführbaren Datei. Es gibt keinen Server, keine Runtime, in die man
  nachträglich Code laden kann.
- **Local-first & sicherheitsbewusst**: Command-History ist opt-in und wird als
  untrusted behandelt; SSH-RC-Injection ist opt-in; AI-Context wird sanitisiert.
  Siehe `docs/SECURITY_PRINCIPLES.md` und `docs/security-notes.md`.
- **Primary-Plattform Windows 11**, Linux/macOS experimentell.

### 1.2 Was es an Erweiterbarkeit schon gibt

Mimir hat bereits eine **halbe Modul-Infrastruktur** — drei Arten von
"Inhalten" werden zur Laufzeit aus dem User-Config-Verzeichnis geladen:

| Inhalt        | Format     | Quelle                                            | Code-Referenz                       |
|---------------|------------|---------------------------------------------------|-------------------------------------|
| Playbooks     | YAML/JSON  | `UserConfigDir/mimir/playbooks/`                  | `workflow/playbook_store.go`        |
| Workflows     | In-Memory  | `Registry` (per ID registriert)                   | `workflow/registry.go`              |
| Templates     | YAML/JSON  | embedded + `UserConfigDir/mimir/templates/`       | `template/template.go`              |

Das Muster ist überall gleich:

- Es gibt **Defaults** (mitgeliefert, geschützt), erkennbar an
  `PlaybookStore.IsProtectedID()`.
- Es gibt **User-Inhalte** (anlegbar, editierbar, löschbar).
- Geladen wird über `os.UserConfigDir()` + Unterordner.

### 1.3 Was *nicht* dynamisch ist: Tools

**Tools** (die eigentlichen Executors) sind Go-Interfaces, die fest
einkompiliert werden:

```go
// tools/types.go
type Tool interface {
    ID() string
    Name() string
    Description() string
    Category() string
    Risk() RiskLevel        // low | medium | high
    Class() ToolClass       // safe_readonly | sensitive_readonly | mutating | destructive | secret_access
    Parameters() []Parameter
    Run(ctx RunContext, input map[string]string) (ToolResult, error)
}
```

Workflow-Schritte (`workflow/types.go`, `StepRunTool`) **referenzieren** Tools
nur über ihre ID — sie bringen keinen eigenen Executor mit. Das ist der
springende Punkt für das ganze Sicherheitsmodell (siehe Abschnitt 4).

---

## 2. Die zentrale Designentscheidung

Bevor irgendetwas gebaut wird, muss **eine** Frage beantwortet werden:

> **Darf ein Modul nur Daten liefern, oder auch Code?**

| Variante                        | Beispiele                                              | Konsequenz                                                                 |
|---------------------------------|--------------------------------------------------------|---------------------------------------------------------------------------|
| **Nur Daten (deklarativ)**      | Playbooks, Workflows, Templates, AI-Prompts            | Sicher, einfach, passt 1:1 auf bestehende Architektur                      |
| **Auch Code (Executors)**       | eigene Tools, eigene Discovery-Resolver                | Mächtig, aber sprengt Single-Binary *und* Sicherheitsmodell               |

**Empfehlung: deklarative Module (nur Daten).**

Begründung im Detail in Abschnitt 4. Kurzfassung: Mimirs gesamte
Sicherheitsgarantie (Risk-Levels, ToolClass, Approval-Gates,
sanitisierter AI-Context) baut darauf, dass die *Executors* vertrauenswürdig und
einkompiliert sind. Ein Modul, das nur vorhandene Tools *referenziert*, läuft
weiterhin vollständig durch die Approval-/Risk-Pipeline. Ein Modul, das eigenen
Code mitbringt, hebelt all das aus.

Der Rest des Dokuments setzt diese Entscheidung voraus, behandelt Code-Module
aber bewusst als optionale Stufe 3 (Abschnitt 7).

---

## 3. Stufe 1 — Das Module-Pack-Format (Fundament)

Ein **Modul** ist ein Verzeichnis (oder eine `.zip`/`.tar.gz`) mit einem
Manifest an der Wurzel und den mitgelieferten Inhalten in Unterordnern.

### 3.1 Manifest

```yaml
# module.yaml
id: net-debug-pack                 # global eindeutig, kebab-case
name: Network Debugging Pack
version: 1.2.0                      # SemVer
author: andlouda
description: >
  Playbooks und Workflows für Netzwerk-Diagnose: Ping-Sweeps,
  DNS-Checks, Portscans (read-only).
homepage: https://github.com/andlouda/mimir-net-debug-pack
license: MIT

# Kompatibilitäts-Gate gegen die App-Version (version.go)
mimirVersion: ">=0.1.0 <0.3.0"

# Was das Modul bereitstellt — relative Glob-Pfade innerhalb des Pakets
provides:
  playbooks:  [playbooks/*.yaml]
  workflows:  [workflows/*.yaml]
  templates:  [templates/*.yaml]

# Welche bereits vorhandenen (einkompilierten) Tools die Inhalte voraussetzen.
# Reine Deklaration zur Validierung — das Modul liefert KEINE Tools.
requiresTools:
  - run_tool:ping
  - run_discovery:netstat

# Integritäts- und Vertrauensmetadaten (vom Marketplace gesetzt)
checksum: "sha256:3b1f...e9a2"     # über den kanonischen Paketinhalt
signature: "..."                   # optional, Ed25519 über den checksum
```

### 3.2 Paket-Layout

```
net-debug-pack/
├── module.yaml
├── playbooks/
│   ├── ping-sweep.yaml
│   └── dns-check.yaml
├── workflows/
│   └── triage-network.yaml
└── templates/
    └── show-routes.yaml
```

### 3.3 Installationsziel

Installiert wird nach:

```
UserConfigDir/mimir/modules/<module-id>/
```

Parallel dazu ein zentrales Verzeichnis-Manifest, das den installierten Zustand
hält (welche Module, welche Version, enabled/disabled):

```
UserConfigDir/mimir/modules/installed.json
```

```json
{
  "modules": [
    {
      "id": "net-debug-pack",
      "version": "1.2.0",
      "enabled": true,
      "installedAt": "2026-06-05T18:00:00Z",
      "source": "registry:official",
      "checksum": "sha256:3b1f...e9a2"
    }
  ]
}
```

### 3.4 Wie Inhalte einhängen — ID-Namespacing

Das wichtigste Detail. Modul-Inhalte dürfen **niemals** mit geschützten
Defaults oder mit anderen Modulen kollidieren. Lösung: jede ID eines
Modul-Inhalts wird mit der Modul-ID präfixiert.

```
playbook:ping-sweep        →  module:net-debug-pack/playbook:ping-sweep
```

Die bestehenden Stores bekommen je eine zusätzliche **Quelle** ("module")
neben "default" und "user". Konkret:

- `PlaybookStore` (`workflow/playbook_store.go`): `List()` aggregiert künftig
  Defaults + User-Dateien + aktivierte Modul-Playbooks. `IsProtectedID()`
  schützt weiterhin nur die Defaults; Modul-Playbooks sind read-only aus
  Sicht des Editors (man editiert sie nicht in-place, man deaktiviert das
  Modul).
- Workflow-`Registry` (`workflow/registry.go`): Modul-Workflows werden beim
  Start / nach Install registriert. `Register()` lehnt heute schon doppelte
  IDs ab — durch das Namespacing kann es keine Kollision mit Defaults geben.
- `template.Manager` (`template/template.go`): analog eine dritte Quelle neben
  embedded und user-dir.

### 3.5 Neues Package: `module/`

Analog zu `workflow/` ein eigenes Package mit klarer Verantwortung:

```
module/
├── manifest.go        // Parsen + Struct des module.yaml
├── validate.go        // Schema-, Version- und requiresTools-Validierung
├── store.go           // ModuleStore: install/remove/enable/disable/list
├── install.go         // Entpacken, Checksum-Verify, atomic write (safeio)
└── *_test.go
```

Skizze des `ModuleStore`:

```go
package module

type InstalledModule struct {
    ID          string    `json:"id"`
    Version     string    `json:"version"`
    Enabled     bool      `json:"enabled"`
    InstalledAt time.Time `json:"installedAt"`
    Source      string    `json:"source"`
    Checksum    string    `json:"checksum"`
}

type ModuleStore struct {
    dir string // UserConfigDir/mimir/modules
}

func NewDefaultModuleStore() (*ModuleStore, error)

// Liest installed.json
func (s *ModuleStore) List() ([]InstalledModule, error)

// Verifiziert Checksum/Signatur, validiert Manifest + Inhalte,
// entpackt atomar nach modules/<id>/, aktualisiert installed.json.
func (s *ModuleStore) Install(pkg io.Reader, expected Trust) (InstalledModule, error)

func (s *ModuleStore) Enable(id string) error
func (s *ModuleStore) Disable(id string) error
func (s *ModuleStore) Remove(id string) error

// Lädt + parst alle aktivierten Manifeste; von den Stores genutzt,
// um ihre "module"-Quelle zu speisen.
func (s *ModuleStore) LoadEnabled() ([]LoadedModule, error)
```

Schreibvorgänge nutzen `safeio.AtomicWriteFile` (wie der `PlaybookStore`), damit
ein abgebrochener Install nichts halb-kaputtes hinterlässt.

---

## 4. Sicherheitsmodell (warum "nur Daten")

Mimirs Sicherheitsversprechen hängt an drei Dingen, die alle voraussetzen, dass
**Executors vertrauenswürdig sind**:

1. **Risk-Levels & ToolClass** (`tools/types.go`): jeder Tool kennzeichnet sich
   als `safe_readonly` … `destructive` / `secret_access`. Approval-Logik
   (`workflow/approval.go`) entscheidet darauf basierend.
2. **Approval-Gates**: Schritte mit `RequiresApproval` pausieren und warten auf
   User-Bestätigung (`workflow.PendingApproval`).
3. **Sanitisierter AI-Context** und deterministische Guardrails
   (`aiflow/guardrails.go`, `docs/ai-deterministic-guardrails.md`).

Ein **deklaratives Modul** kann diese Garantien nicht aushebeln:

- Es referenziert nur vorhandene Tools per ID → läuft durch dieselbe Risk-/
  Approval-Pipeline.
- Ein bösartiges Playbook kann höchstens *existierende* Tools in ungünstiger
  Reihenfolge aufrufen — und genau dafür gibt es die Approval-Gates und die
  Class-Prüfung.

Ein **Code-Modul** würde dagegen einen neuen Executor einschleusen, der sich
sein eigenes (zu niedriges) Risk-Level geben oder die Guardrails komplett
umgehen könnte. Damit wäre jede Garantie wertlos.

### 4.1 Zusätzliche Schutzmaßnahmen für Stufe 1

Auch deklarative Module brauchen Validierung — ein Modul ist untrusted Input:

- **Manifest-Validierung** (analog `workflow/validate.go`): Pflichtfelder,
  SemVer-Format, erlaubte Glob-Pfade (kein `../`-Ausbruch, keine absoluten
  Pfade), Größenlimits.
- **`requiresTools`-Check**: Bei Install/Enable prüfen, ob alle referenzierten
  Tool-IDs in der einkompilierten Tool-Registry existieren. Fehlt eine → Modul
  wird als "incompatible" markiert statt still zu laufen.
- **Inhalts-Validierung**: jedes mitgelieferte Playbook/Workflow/Template läuft
  durch denselben Validator wie User-Inhalte. Keine Sonderbehandlung.
- **Checksum-Verify** vor dem Entpacken; **Signaturprüfung** optional (siehe
  5.3).
- **Capability-Anzeige in der UI**: vor Install dem User zeigen, welche Tools
  (und welche Risk-Klassen) das Modul nutzt — Transparenz statt blindem
  Vertrauen.

---

## 5. Stufe 2 — Marketplace (Distribution)

Kein eigener Server nötig. Local-first-freundlich und konsistent mit dem schon
vorhandenen Update-Mechanismus (`app_update.go` nutzt GitHub Releases).

### 5.1 Git/GitHub-basierter Registry-Index

Ein öffentliches Repo `mimir-registry` mit einer `index.json`:

```json
{
  "schemaVersion": 1,
  "updatedAt": "2026-06-05T00:00:00Z",
  "modules": [
    {
      "id": "net-debug-pack",
      "name": "Network Debugging Pack",
      "description": "Read-only Netzwerk-Diagnose.",
      "author": "andlouda",
      "latest": "1.2.0",
      "mimirVersion": ">=0.1.0",
      "tags": ["network", "diagnostics"],
      "downloadUrl": "https://github.com/andlouda/mimir-net-debug-pack/releases/download/v1.2.0/net-debug-pack-1.2.0.zip",
      "checksum": "sha256:3b1f...e9a2",
      "signature": "..."
    }
  ]
}
```

Ablauf:

1. App lädt `index.json` (gecached, mit ETag).
2. UI zeigt durchsuchbare Liste, gefiltert nach `mimirVersion`-Kompatibilität.
3. Install: `downloadUrl` ziehen → Checksum/Signatur prüfen → `ModuleStore.Install`.
4. Update: Index-`latest` mit installierter Version vergleichen.

Vorteile: kein Hosting-Aufwand, alles über GitHub Releases (genau wie heute die
App-Updates), PRs gegen das Registry-Repo als Einreichungs-Prozess,
Git-History als Audit-Trail.

### 5.2 Quellen / Trust-Tiers

`installed.json.source` hält fest, woher ein Modul kam:

- `registry:official` — kuratiert, signiert.
- `registry:community` — gelistet, evtl. unsigniert.
- `url:<host>` — direkt von einer URL.
- `local` — lokale Datei (Entwicklung / privat).

Die UI markiert nicht-offizielle Quellen deutlich und verlangt eine explizite
Bestätigung.

### 5.3 Signaturen (optional, empfohlen für "official")

- Offizielle Module mit einem Ed25519-Key signieren; Public Key in die App
  einkompilieren.
- Bei Install: Signatur über den Checksum prüfen.
- Community-Module dürfen unsigniert sein, werden aber im Trust-Tier
  niedriger eingestuft und in der UI entsprechend gekennzeichnet.

---

## 6. UI / Frontend

Ein neues **"Modules"**-Panel in der Sidebar, konsistent mit den bestehenden
Svelte-Panels (`frontend/src/lib/*.svelte`, z. B. `Sidebar.svelte`,
`AppModals.svelte`).

Funktionen:

- **Browse**: Registry-Index durchsuchen, Detailseite je Modul (Beschreibung,
  Autor, Version, benötigte Tools/Risk-Klassen, Quelle/Trust-Tier).
- **Install / Update / Remove**.
- **Enable / Disable** ohne Deinstallation.
- **Installed**-Tab: lokal vorhandene Module verwalten, "Update verfügbar"-Badge.
- **Capability-Dialog** vor Install: "Dieses Modul nutzt: ping (safe_readonly),
  netstat (sensitive_readonly). Fortfahren?"

Backend-Anbindung über Wails-Bindings (ein neues `app_modules.go` analog zu
`playbooks_api.go`), das `ModuleStore` und den Registry-Client nach vorn
exponiert.

---

## 7. Stufe 3 — Code-Module (bewusst später / optional)

Falls eines Tages echte Code-Erweiterung gebraucht wird, hier die realistischen
Optionen — mit klaren Nachteilen:

| Ansatz                         | Bewertung                                                                 |
|--------------------------------|---------------------------------------------------------------------------|
| Go `plugin` package            | **Raus.** Unter Windows (Primary!) nicht unterstützt.                      |
| Out-of-process (JSON-RPC/gRPC) | Wie hashicorp/go-plugin oder LSP: Modul ist eigener Prozess, Mimir spricht über ein definiertes Protokoll. Sandboxing per OS-Mitteln. Viel Infrastruktur. |
| WASM (z. B. wazero)            | Sandboxed Tool-Runtime, deterministisch, plattformübergreifend. Eleganteste Variante, aber neues Tool-Interface + Host-Bindings nötig. |

Beide gangbaren Varianten (Out-of-process, WASM) durchlöchern das "alles
auditierbar, Executors vertrauenswürdig"-Modell und sind erheblich mehr Arbeit.
**Empfehlung: erst bauen, wenn es eine konkrete Nachfrage gibt, die sich nicht
deklarativ lösen lässt.** Wenn doch, dann WASM bevorzugen — es bleibt sandboxed
und plattformneutral, was zu Mimirs Sicherheitsanspruch passt.

---

## 8. Phasenweiser Umsetzungsplan

### Phase 0 — Entscheidung
- [ ] Festlegen: Module nur Daten (empfohlen) oder auch Code.
- [ ] Festlegen: Welche Inhaltstypen in v1 (mindestens Playbooks; Workflows +
      Templates optional).

### Phase 1 — Fundament (`module/` Package)
- [ ] `module.yaml`-Schema + `manifest.go` (Parsing).
- [ ] `validate.go`: Manifest-, Pfad- (kein `../`), Version-, `requiresTools`-
      Validierung.
- [ ] `ModuleStore` (`store.go`, `install.go`): install/remove/enable/disable,
      `installed.json`, atomare Writes via `safeio`, Checksum-Verify.
- [ ] Unit-Tests analog zu `workflow/*_test.go`.

### Phase 2 — Integration in die Stores
- [ ] ID-Namespacing (`module:<id>/...`) einführen.
- [ ] `PlaybookStore.List()` um Modul-Quelle erweitern (read-only, nicht in
      `IsProtectedID`, aber editiergeschützt).
- [ ] Workflow-`Registry` beim Start/nach Install mit Modul-Workflows speisen.
- [ ] `template.Manager` um Modul-Quelle erweitern.
- [ ] Reload-Pfad: nach Enable/Disable/Install Stores neu laden, ohne Neustart.

### Phase 3 — Backend-API
- [ ] `app_modules.go` mit Wails-Bindings: List/Install/Remove/Enable/Disable,
      Registry-Index laden.
- [ ] Registry-Client (`index.json` laden, ETag-Cache, Kompatibilitätsfilter).

### Phase 4 — Frontend
- [ ] "Modules"-Panel (Browse / Installed).
- [ ] Capability-Dialog vor Install.
- [ ] Trust-Tier-Kennzeichnung, Update-Badges.

### Phase 5 — Marketplace-Repo
- [ ] `mimir-registry` Repo mit `index.json`-Schema + erstem offiziellen Modul.
- [ ] Einreichungs-Prozess (PR-Template, Checks).
- [ ] Optional: Ed25519-Signierung für offizielle Module, Public Key in die App.

---

## 9. Offene Fragen / Risiken

- **Versionierung von Inhalten**: Was passiert mit laufenden Workflows, wenn ein
  Modul mitten im Lauf deaktiviert/aktualisiert wird? → Modul-Inhalte beim
  Start eines Laufs in den State kopieren, nicht zur Laufzeit nachladen.
- **Konflikte bei `requiresTools`**: Tool wurde in neuerer App-Version
  umbenannt/entfernt → Modul als "incompatible" markieren statt Fehler beim
  Ausführen.
- **Default-Migration**: Heutige Default-Playbooks (`retiredDefaultIDs` in
  `playbook_store.go`) zeigen, dass Defaults sich ändern. Modul-Inhalte dürfen
  nie als Default gelten — Namespacing schützt davor.
- **Update-Strategie**: Auto-Update von Modulen (wie App-Updates) vs. nur
  manuell? Für Sicherheit eher manuell mit sichtbarem Diff/Capability-Hinweis.

---

## 10. Zusammenfassung

- Mimir hat bereits das Fundament für deklarative Erweiterungen
  (Playbooks/Workflows/Templates aus dem Config-Dir).
- Der pragmatische, sichere Weg ist ein **deklaratives Modul-Format** (nur
  Daten, keine Executors), das die bestehenden Stores als zusätzliche Quelle
  speist — mit striktem ID-Namespacing und Validierung.
- Ein **Marketplace** lässt sich ohne eigenen Server über ein
  GitHub-Registry-Repo mit `index.json` realisieren, konsistent zum bestehenden
  Update-Mechanismus.
- **Code-Module** (WASM/Out-of-process) sind technisch möglich, aber teuer und
  sicherheitskritisch — bewusst auf später verschoben.
