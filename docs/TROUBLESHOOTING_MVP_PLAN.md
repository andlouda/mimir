# Troubleshooting MVP Plan

## Phase 1: Discovery Visible And Useful

Ziel:

- Discovery nicht nur implizit im Guardrail-Pfad, sondern sichtbar in der App

Umfang:

- Discovery-Funktionen im Function Catalog
- Discovery Preview aus der UI
- read-only Resolver für K8s / Docker / Compose
- Audit-Events für Discovery

Erfolg:

- User kann echte Namespaces, Pods, Container und Services in der App sehen

## Phase 2: Troubleshooting Playbooks

Ziel:

- die wichtigsten Diagnosepfade als benannte Standardabläufe verfügbar machen

Erste Playbooks:

- Host Basic Triage
- Docker Compose Debug
- Kubernetes Pod Triage
- API / Network Health Check

Erfolg:

- User muss nicht jeden Ablauf manuell zusammensetzen

## Phase 3: Approval And Replay

Ziel:

- jede AI-Aktion nachvollziehbar und prüfbar machen

Umfang:

- Prompt-/Kontext-Zusammenfassung
- Discovery-Ergebnisse pro Run
- sichtbarer Block-/Allow-Grund
- Replay-/Audit-Ansicht

Erfolg:

- AI-Verhalten ist nicht nur nützlich, sondern vertrauenswürdig

## Phase 4: Workflow Runner

Ziel:

- Builder, Engine und Approval wirklich zusammenführen

Umfang:

- Draft ans Backend senden
- Step-Ausführung
- Pause / Resume / Approval
- Event-/Log-Ansicht

Erfolg:

- aus Planung wird reale, kontrollierte Troubleshooting-Orchestrierung

## Phase 5: Agent Context

Ziel:

- Projekt- und Agent-Dateien als sichere Kontextquellen nutzbar machen

Umfang:

- `AGENTS.md`
- `README.md`
- `SKILL.md`
- Kontext-Vorschau
- Redaction
- bewusste User-Auswahl

Erfolg:

- AI kann projekt- und umgebungsspezifischer helfen, ohne blind zu lesen

## Erste Umsetzungseinheiten

Die ersten konkreten Einheiten für die aktuelle Entwicklung sind:

1. Discovery-Resolver im Backend
2. Discovery-Funktionen im Function Catalog
3. Discovery Preview im UI
4. Discovery-Parameter an K8s-/Docker-Templates
5. Troubleshooting-Playbook-Definitionen als nächste Daten-/UI-Schicht
