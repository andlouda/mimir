# Security Principles

Diese Datei beschreibt die Sicherheits- und Kontrollprinzipien für Mimir als AI-gestützte Terminal- und Workflow-App.

## Zielbild

Mimir soll **nicht** einfach ein Chatfenster über einem Terminal sein.

Mimir soll:

- AI-unterstützt sein
- deterministisch abgesichert sein
- nachvollziehbar sein
- den User am Fahrerhebel lassen

Die Kernidee ist:

**AI assists. The user authorizes. The system enforces.**

## 1. User In Control

Der Benutzer bleibt die steuernde Instanz.

Das bedeutet:

- AI darf vorbereiten, erklären, vorschlagen und entdecken
- AI darf nicht frei unkontrolliert handeln
- riskante oder mutierende Aktionen brauchen explizite Freigabe
- der User muss sehen können, was wirklich ausgeführt werden soll

Mimir optimiert auf:

- Kontrolle
- Transparenz
- Verlässlichkeit

nicht auf:

- maximale Autonomie um jeden Preis

## 2. Deterministic Guardrails Over Prompt-Only Safety

Prompt-Text allein ist keine Sicherheitsgrenze.

Deshalb gilt:

- Guardrails dürfen nicht nur im Prompt stehen
- zentrale Regeln müssen backendseitig erzwungen werden
- editierbare User-Prompts dürfen System-Guardrails nicht überschreiben

Beispiele:

- nur registrierte Tool-IDs
- kein freier Shell-Command-Fallback
- feste Blocklisten für destruktive Muster
- feste Output-Validierung
- feste Parameter-Validierung

## 3. Discovery Before Execution

Kritische Zielwerte sollen nicht halluziniert werden.

Beispiele:

- Pod-Namen
- Namespace-Namen
- Container-Namen
- Compose-Service-Namen

Darum gilt:

- AI soll reale Zielobjekte zuerst discovern
- nur Werte aus echten Discovery-Ergebnissen dürfen weiterverwendet werden
- Discovery ist read-only

Das reduziert:

- Halluzinationen
- Fehlbedienung
- falsche Zielauswahl

## 4. Least Privilege By Default

AI bekommt nur so viel Zugriff wie nötig.

Das bedeutet:

- standardmäßig read-only
- Cloud-Provider strenger als lokale Provider
- Tool-Klassen mit klarer Policy
- sensitive und mutierende Bereiche nicht einfach pauschal freigeben

Beispielrichtung:

- `openai`: nur `safe_readonly`
- `ollama`: kontrolliert offener, aber weiterhin abgesichert

## 5. Human Approval For Risk

Mutation, Side Effects und Unsicherheit brauchen menschliche Bestätigung.

Darum gilt:

- read-only kann je nach Policy direkt laufen
- `medium` und `high` risk brauchen Approval
- unklare oder ambige Fälle sollen blockieren statt raten

Mimir bevorzugt:

- konservatives Blocken

gegenüber:

- unsicherem “wird schon passen”

## 6. Context Minimization And Redaction

Nicht jeder verfügbare Kontext darf an ein Modell gesendet werden.

Darum gilt:

- nur begrenzter Terminal-Kontext
- ANSI-Stripping
- Secret-Redaction
- sensitive Zeilen bei strikten Providern entfernen
- keine unnötige Exfiltration

Der Grundsatz lautet:

**Nur der Kontext, der wirklich gebraucht wird, darf das Modell erreichen.**

## 7. Reproducibility And Auditability

Eine AI-Aktion muss nachvollziehbar sein.

Darum will Mimir protokollieren und sichtbar machen:

- welcher Prompt effektiv verwendet wurde
- welcher Kontext einfloss
- was redigiert wurde
- welche Tools sichtbar waren
- welche Discovery-Ergebnisse vorlagen
- warum etwas erlaubt oder blockiert wurde
- was tatsächlich ausgeführt wurde

Das Ziel ist:

- Debugbarkeit
- Vertrauen
- sichere Betriebsführung

## 8. Explainability Over Magic

Mimir soll erklärbar sein, nicht mystisch.

Wenn etwas blockiert wird, soll die App sagen:

- welche Regel gegriffen hat
- warum die Aktion nicht erlaubt ist
- was stattdessen möglich wäre

Wenn etwas ausgeführt wird, soll klar sein:

- welches Tool
- mit welchen Inputs
- unter welcher Policy

## 9. Modularity As A Security Feature

Sicherheitslogik soll nicht als verstreute Sonderfälle im Code leben.

Darum wird Mimir modular aufgebaut:

- Tools
- Workflows
- Approval Policy
- Context Sanitizer
- Discovery Resolver
- Output Validator
- Audit Log

Das verbessert:

- Lesbarkeit
- Testbarkeit
- Erweiterbarkeit
- Sicherheitsreview

## 10. Do Not Rebuild Claude

Mimir soll nicht versuchen, bestehende Agent-Tools blind nachzubauen oder zu reverse engineeren.

Stattdessen:

- auf etablierte Konzepte aufbauen
- Standards respektieren
- dort differenzieren, wo Mimir echten Mehrwert bringt

Der Mehrwert liegt in:

- deterministischer Security
- klaren Guardrails
- Workflow-Orchestrierung
- User-Kontrolle
- Auditierbarkeit

## Produktlinie

Die Produktlinie für Mimir lautet:

- `AI assists, user authorizes`
- `Discovery before execution`
- `Immutable guardrails over editable prompts`
- `Every action is explainable and replayable`
- `Cloud strict, local controlled-flexible`

## Konsequenz Für Neue Features

Neue AI- oder Workflow-Features sollten diese Fragen bestehen:

1. Bleibt der User am Fahrerhebel?
2. Gibt es deterministische Durchsetzung statt nur Prompt-Instruktionen?
3. Ist klar, welche Daten ans Modell gehen?
4. Ist nachvollziehbar, warum etwas erlaubt oder blockiert wurde?
5. Ist die Aktion auditierbar und reproduzierbar?
6. Passt die Funktion in ein modulares Sicherheitsmodell?

Wenn eine neue Funktion diese Fragen nicht gut beantwortet, sollte sie nicht direkt eingebaut, sondern zuerst architektonisch nachgeschärft werden.
