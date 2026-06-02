# template

## Zweck
Verwaltung von Befehlsvorlagen (Templates) für verschiedene Terminal-Typen. Templates ermöglichen wiederverwendbare Befehle mit Variablensubstitution.

## Inhalt
- `template.go` - Template-Datenmodell, Manager (CRUD + Apply), Filename-Sanitierung
- `template_test.go` - Unit Tests (39.4% Coverage)

## Verantwortlichkeiten
- Laden von Templates aus einem `fs.FS` (z.B. eingebettete Dateien via `go:embed`)
- CRUD-Operationen für Templates (Create, Read, Update, Delete)
- Template-Anwendung: Variablensubstitution via Go `text/template` und Schreiben an einen `Writer`
- Favoriten-Management (Toggle)
- Filename-Sanitierung zum Schutz vor Path-Traversal

**Nicht hierhin gehört:**
- PTY-Management (siehe `terminal/`)
- UI-Darstellung (siehe Frontend `TemplateManager.svelte`)

## Abhängigkeiten
- `io/fs` - Filesystem-Abstraktion für Template-Laden
- Wird von `app.go` verwendet
- Keine externe Dependencies

## Wichtige Hinweise
- **Writer Interface:** `template.Writer` entkoppelt die Template-Ausführung von ConPTY. Jeder Typ mit `Write([]byte) (int, error)` ist kompatibel.
- **fs.FS statt embed.FS:** Ermöglicht Unit Tests mit `testing/fstest.MapFS`.
- **Speicherpfade:** Eingebettete Templates werden read-only geladen. User-Templates und Overrides werden im privaten User-Config-Verzeichnis unter `mimir/templates` gespeichert.
- **Sicherheitshinweis:** Template-Befehle werden als Go-Templates geparst (`text/template`). Variablenwerte werden vor dem Schreiben an das Terminal gegen deklarierte Parameterregeln oder einen konservativen Shell-Atom-Fallback validiert.
- **Dateiformat:** JSON mit Feldern `name`, `description`, `commands` (Map: terminalType -> command), `favorite`.

## Beispiele

```go
// Manager erstellen und Templates laden
m := template.NewManager(embeddedFS)
err := m.LoadTemplates()

// Template anwenden
ctx := template.TemplateContext{
    CurrentDir: "/home/user",
    Username:   "admin",
}
err = m.ApplyTemplate(terminalID, "List Files", "bash", ptyWriter, ctx)
```
