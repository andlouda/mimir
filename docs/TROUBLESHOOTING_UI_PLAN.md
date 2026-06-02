# Troubleshooting UI Plan

## Ziel

Die aktuelle Workflow-/Playbook-/Discovery-Oberfläche ist funktional, aber zu technisch.

Der Nutzer soll **nicht** zuerst interne Begriffe wie:

- `run_tool`
- `run_discovery`
- `ask_ai`
- JSON-Draft

verstehen müssen.

Die UI soll stattdessen auf einen klaren Troubleshooting-Flow optimiert werden:

1. Problemkontext wählen
2. passendes Playbook starten
3. Discovery-Ergebnisse sehen
4. sichere Schritte ausführen
5. AI-Erklärungen lesen
6. Audit / Replay nachvollziehen

## Leitprinzipien

### 1. Playbooks first

Der Haupteinstieg soll über **Playbooks** laufen, nicht über den generischen Workflow-Builder.

Der Nutzer soll zuerst sehen:

- `Docker Compose Debug`
- `K8s Pod Triage`
- `Host Basic Triage`
- `API / Network Health Check`

Der freie Builder bleibt vorhanden, aber als erweiterter Modus.

### 2. Troubleshooting statt Workflow-Technik

Die Oberfläche soll die Sprache des Anwendungsfalls sprechen:

- `Inspect`
- `Discover`
- `Explain`
- `Next safe step`

nicht:

- `step type`
- `tool id`
- `discoveryTool`

### 3. User stays in control

Die UI muss klar machen:

- was automatisch nur gelesen wird
- was AI erklärt
- was ausgeführt wird
- wann Freigabe nötig ist

### 4. Auditability visible

Replay und Nachvollziehbarkeit gehören zur Oberfläche, nicht nur zu Log-Dateien.

## Ziel-Informationsarchitektur

### Sidebar

Empfohlene Hauptnavigation:

1. `Terminals`
2. `Playbooks`
3. `Functions`
4. `Files`
5. `AI`
6. `Logs`

### Bedeutung

- `Playbooks`
  - Haupteinstieg für Troubleshooting
- `Functions`
  - Werkzeugkatalog
  - Discovery, Tools, AI-Aktionen
- `AI`
  - Settings, Policies, Prompt/Runtime, Audit-Zusammenfassung
- `Logs`
  - Replay / Security / Workflow / Tool-Audit

## Zielbild pro Bereich

## 1. Playbooks

### Zweck

Benannte Standardabläufe für bekannte Troubleshooting-Fälle.

### Ziel-UI

Zweiteilung:

- links: Playbook-Library
- rechts: Playbook-Detail / letzter Run

### Playbook Card

Jede Karte zeigt:

- Name
- Kurzbeschreibung
- Scope
- geschätzte Schrittzahl
- benötigter Kontext
  - `Terminal required`
  - `K8s`
  - `Docker`
  - `SSH`
- Risk-Badge
  - `Read-only`
  - `Approval-aware`

### Playbook Detail

Soll zeigen:

- Beschreibung
- typische Einsatzfälle
- welche Discovery gemacht wird
- welche Tools/Schritte enthalten sind
- was AI darin macht
- Start-Button

### Run-Ansicht

Nach dem Start:

- Schrittliste mit Status
  - `pending`
  - `running`
  - `completed`
  - `blocked`
- Discovery-Ergebnisse
- Tool-Outputs
- AI-Explanation
- `Approve` / `Deny` falls nötig
- `Replay in Logs`

## 2. Functions

### Zweck

Alle Bausteine sichtbar und erklärbar machen.

### Ziel-UI

Der bisherige Function Catalog wird in eine vollwertige Werkzeugübersicht überführt.

### Bereiche

- `Discovery`
- `Read-only Tools`
- `AI Actions`
- später optional:
  - `Human-only Tools`

### Function Detail

Soll zeigen:

- Name
- Beschreibung
- Kategorie
- Class / Risk
- benötigte Parameter
- ob Discovery nötig ist
- ob AI diese Funktion nutzen darf
- unterstützte Umgebungen

### Aktionen

Je nach Funktion:

- `Run Discovery`
- `Add to Playbook / Workflow`
- `Ask AI what this does`

## 3. Custom Workflow Builder

### Zweck

Advanced Mode für eigene Abläufe.

### Positionierung

Nicht mehr der primäre Einstieg.

### Umbenennung

Der Bereich sollte sichtbarer als:

- `Custom Workflow`

oder

- `Advanced Builder`

gekennzeichnet werden.

### Ziel-UI

Tabs:

1. `Playbooks`
2. `Custom Builder`

### Builder-Vereinfachung

Nicht primär JSON zeigen.

Stattdessen:

- visuelle Schrittkarten
- Discovery-Schritt
- Tool-Schritt
- AI-Erklärschritt
- Approval-Marker

JSON erst in einem einklappbaren Bereich:

- `Advanced JSON`

Mermaid ebenfalls nur sekundär:

- `Flow Diagram`

## 4. Discovery UX

### Ziel

Discovery muss wie ein erstklassiges Troubleshooting-Feature wirken.

### Ziel-UI

Bei Discovery-Funktionen:

- Eingabefelder für Kontext
- `Run Discovery`
- Ergebnisliste als strukturierte Liste
- Copy / reuse / insert into next step

### Wichtige Folgefunktion

Discovery-Ergebnisse sollen nicht nur angezeigt, sondern weiterverwendet werden:

- Namespace anklicken
- Pod anklicken
- Service anklicken

Dann:

- nächste Felder automatisch vorbelegen
- Playbook-Schritte befüllen

## 5. AI im Troubleshooting

### AI soll in der UI als 3 Arten von Hilfe sichtbar sein

1. `Explain`
2. `Summarize`
3. `Suggest next safe step`

### Nicht als

- freier “Agent macht irgendwas”-Bereich

### Ziel-UI

Im Run-Kontext:

- kompakte AI-Karten unter dem jeweiligen Schritt
- z. B.:
  - `AI Summary`
  - `Likely Cause`
  - `Suggested Next Read-only Step`

## 6. Approval UX

### Ziel

Approval darf nicht nur eine Textmeldung sein.

### Ziel-UI

Eigener Approval-Block mit:

- betroffener Schritt
- Tool / Discovery / Aktion
- Inputs
- Risk
- Begründung
- `Approve`
- `Deny`

### Zusätzlich

- klarer Unterschied zwischen:
  - `read-only`
  - `needs approval`
  - `blocked by policy`

## 7. Logs / Replay

### Ziel

Logs sollen direkt für Troubleshooting-Runs nutzbar sein.

### Ziel-UI

Zusätzliche Filter:

- `Playbook Runs`
- `Discovery`
- `AI Explanations`
- `Blocked by Guardrails`

### Jede Run-Ansicht sollte linkbar sein zu:

- Logs
- Security Events
- AI Interaction

### Replay-Sicht

Eine einzelne Troubleshooting-Session sollte bündeln:

- Playbook
- Inputs
- Discovery-Ergebnisse
- Tool-Schritte
- AI-Antworten
- Approvals

## Konkrete UI-Bausteine

## Neue Komponenten

Empfohlen:

- `lib/playbooks/PlaybookLibrary.svelte`
- `lib/playbooks/PlaybookCard.svelte`
- `lib/playbooks/PlaybookDetail.svelte`
- `lib/playbooks/PlaybookRunner.svelte`
- `lib/playbooks/ApprovalPanel.svelte`
- `lib/functions/FunctionCatalog.svelte`
- `lib/functions/FunctionDetail.svelte`
- `lib/functions/DiscoveryPreview.svelte`
- `lib/workflows/CustomWorkflowBuilder.svelte`

## Zustand / Datenfluss

Benötigte Zustände:

- aktives Playbook
- Run-State
- Discovery-Ergebnisse pro Schritt
- letzter AI-Output pro Schritt
- Approval-Pending-State
- Replay-Referenz / Run-ID

## Umsetzungsreihenfolge

## Phase 1

Playbooks sichtbar priorisieren

- neuer `Playbooks`-Bereich
- Library + Detailansicht
- Playbook-Karten

## Phase 2

Playbook Runner

- Steps sichtbar
- Status sichtbar
- Discovery-Ergebnisse sichtbar
- AI-Ergebnisse sichtbar

## Phase 3

Approval UX

- Approval-Karte
- Freigabe / Ablehnung
- Blocked-States

## Phase 4

Function Catalog sauber trennen

- Discovery / Tools / AI Actions
- bessere Detailansicht

## Phase 5

Custom Workflow Builder vereinfachen

- JSON sekundär
- Playbook-ähnlichere Step-Karten

## Phase 6

Replay / Audit UX

- Run-Historie
- Session-Zusammenfassung
- Link von Playbook zu Logs

## Was im aktuellen Stand irritiert

Die wichtigsten Probleme der aktuellen UI:

1. `Workflow` ist als Begriff zu technisch prominent.
2. Playbooks fühlen sich noch nicht wie das Hauptprodukt an.
3. Discovery ist sichtbar, aber noch nicht elegant in Folgeaktionen eingebettet.
4. JSON-Preview steht zu stark im Vordergrund.
5. Approval/Run-State ist noch nicht als echter Bedienfluss gestaltet.

## Zielzustand in einem Satz

Die UI soll sich anfühlen wie:

**“Ich wähle einen Troubleshooting-Fall, sehe was die App entdeckt, verstehe was passiert, und entscheide bewusst über die nächsten sicheren Schritte.”**
