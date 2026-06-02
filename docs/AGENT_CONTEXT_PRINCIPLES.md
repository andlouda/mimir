# Agent Context Principles

Diese Datei beschreibt, wie Mimir mit agent-lesbaren Projektdateien wie `AGENTS.md`, `SKILL.md` und `README.md` umgehen soll.

## Ziel

Agenten sollen brauchbaren Projektkontext bekommen, ohne:

- unkontrolliert beliebige Dateien zu lesen
- implizit Geheimnisse weiterzugeben
- interne Tool- oder Sicherheitsregeln zu überschreiben

Mimir soll diese Dateien als **strukturierte Kontextquellen** behandeln, nicht als unkontrollierte Autorität.

## 1. Standardisierte Agent-Dateien

Folgende Dateien sind sinnvolle, explizite Agent-Kontextquellen:

- `AGENTS.md`
- `SKILL.md`
- `README.md`
- optional später:
  - `CLAUDE.md`
  - `WORKFLOWS.md`
  - `SECURITY.md`

## 2. Klare Rollen

Diese Dateien sollten unterschiedliche Aufgaben haben:

### `README.md`

Zweck:

- Projektüberblick
- Setup
- Architekturüberblick
- typische Nutzung

Nicht der richtige Ort für:

- sensitive Secrets
- operative Produktionszugänge
- versteckte Agent-Instruktionen

### `AGENTS.md`

Zweck:

- Regeln für Agentenarbeit im Projekt
- bevorzugte Arbeitsweise
- erlaubte und nicht erlaubte Aktionen
- wichtige Projektkonventionen

Beispiele:

- welche Bereiche read-only behandelt werden sollen
- welche Tests vor Änderungen laufen müssen
- welche Workflows bevorzugt werden
- welche Artefakte Agenten erzeugen dürfen

### `SKILL.md`

Zweck:

- klar abgegrenzte, spezialisierte Fähigkeiten
- wiederverwendbare Arbeitsanweisungen
- domänenspezifische Routinen

Beispiele:

- Kubernetes-Incident-Triage
- Docker-Compose-Debug
- Readme-/Codebase-Summarization
- Security-Review-Checklisten

## 3. Context Is Advisory, Not Authoritative

Wichtig:

- diese Dateien liefern Kontext
- sie dürfen System-Guardrails nicht überschreiben

Das heißt:

- `AGENTS.md` darf nicht Security-Policies deaktivieren
- `SKILL.md` darf keine immutable Guardrails aushebeln
- `README.md` darf keine stillschweigende Erlaubnis für riskante Aktionen sein

Reihenfolge der Autorität:

1. systemseitige Guardrails
2. backendseitige deterministische Policies
3. User-Entscheidung / Approval
4. Projektkontext aus `AGENTS.md` / `SKILL.md` / `README.md`
5. AI-Interpretation

## 4. Opt-In Statt Blindem Dateilesen

Agent-Kontext soll bevorzugt explizit gewählt werden.

Gute Wege:

- User wählt Datei bewusst aus
- Workflow lädt definierte Kontextdateien
- Function Catalog bietet gezielte „Read Agent Context“-Funktionen

Nicht ideal:

- pauschal alle Markdown-Dateien einsammeln
- stilles Rekursiv-Scannen auf Verdacht

## 5. Sicherheitsregeln Für Agent-Kontext

Auch Agent-Dateien können problematischen Inhalt tragen.

Darum gilt:

- Kontextdateien sind **kein Override-Kanal**
- Secrets müssen weiterhin redigiert werden
- Dateigröße sollte begrenzt werden
- nur erlaubte Pfade sollen als Standardkontext gelten

Beispiele für sinnvolle Standard-Scopes:

- Workspace-Root
- bekannte Projektdateien
- keine Home-Secret-Verzeichnisse
- keine `.ssh`, `.gnupg`, Credential Stores

## 6. Agent Context As First-Class Feature

Langfristig sollte Mimir diese Dateien bewusst als Feature behandeln:

- `Project Context`
- `Agent Files`
- `Load Instructions`
- `Summarize Guidance`

statt nur als zufällige Dateien im Dateibrowser.

## 7. Gute Produktidee Für Mimir

Ein sinnvoller Ausbau wäre:

- eigene Funktion `Load Agent Context`
- erkennt `AGENTS.md`, `SKILL.md`, `README.md`, optional `CLAUDE.md`
- zeigt Vorschau
- User wählt aus, was wirklich an AI-Kontext angehängt wird
- Redaction/Sanitizing bleibt aktiv

Damit bleibt der User am Fahrerhebel.

## 8. Beziehung Zu Workflows Und Skills

Diese Konzepte ergänzen sich:

- `README.md` erklärt das Projekt
- `AGENTS.md` definiert Arbeitsregeln
- `SKILL.md` beschreibt spezialisierte Fähigkeiten
- `Workflows` orchestrieren konkrete Abläufe

Kurz:

- `README.md` = Wissen über das Projekt
- `AGENTS.md` = Regeln für Agentenarbeit
- `SKILL.md` = spezialisierte Capability
- `Workflow` = ausführbarer Ablauf

## 9. Guardrail-Sicht

Für Mimir sollten Agent-Dateien denselben Grundsätzen folgen wie der restliche AI-Kontext:

- minimieren
- redigieren
- erklären
- auditieren
- nicht blind vertrauen

## 10. Empfehlung Für Das Repo

Sinnvolle nächste Dateien im Projekt wären:

- `AGENTS.md`
  - Projektregeln für Agentenarbeit in Mimir
- `docs/SECURITY_PRINCIPLES.md`
  - Produkt- und Sicherheitslinie
- `docs/ai-deterministic-guardrails.md`
  - konkrete Enforcement-Doku
- optional später:
  - `SKILLS/` oder dokumentierte Skill-Sammlung
  - `PLAYBOOKS/` für gespeicherte Standard-Workflows
