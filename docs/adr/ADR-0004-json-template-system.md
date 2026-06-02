# ADR-0004: JSON-basiertes Template-System fuer Multi-Shell-Befehle

## Status

Angenommen

## Kontext

Mimir soll haeufig verwendete Befehle als wiederverwendbare Templates bereitstellen. Ein Template muss denselben logischen Befehl (z.B. "Dateien auflisten") fuer verschiedene Shell-Typen (cmd, powershell, bash, wsl, zsh) abbilden koennen, da der Benutzer mehrere Terminal-Typen gleichzeitig offen hat.

Zusaetzlich sollen Templates dynamische Variablen unterstuetzen (aktuelles Verzeichnis, Benutzername, Hostname), um kontextabhaengige Befehle zu generieren.

Die Templates werden sowohl als eingebettete Standard-Templates (im Binary enthalten) als auch als benutzerdefinierte Templates (im Dateisystem gespeichert) benoetigt.

## Entscheidung

Jedes Template wird als einzelne JSON-Datei im Verzeichnis `templates/` gespeichert. Die Struktur ist:

```json
{
  "name": "List Files",
  "description": "Lists files in the current directory.",
  "commands": {
    "bash": "ls -la",
    "cmd": "dir",
    "powershell": "Get-ChildItem",
    "wsl": "ls -la",
    "zsh": "ls -la"
  },
  "favorite": false
}
```

Die `commands`-Map ordnet jedem Shell-Typ den passenden Befehl zu. Befehle koennen Go-Template-Variablen enthalten (z.B. `cd {{.CurrentDir}}`), die ueber `text/template` aufgeloest werden.

In `main.go` werden die Templates ueber `//go:embed templates` in die Binary eingebettet. Der `template.Manager` liest diese ueber `fs.ReadDir` und `fs.ReadFile` aus dem uebergebenen `fs.FS`.

`ApplyTemplate()` sucht das passende Template, waehlt den Befehl fuer den aktuellen Shell-Typ aus, fuehrt Go-Template-Substitution mit dem `TemplateContext` durch und schreibt das Ergebnis mit `\r\n`-Suffix an den PTY-Writer.

Benutzer koennen neue Templates ueber `SaveTemplate()`, vorhandene ueber `UpdateTemplate()` aendern, ueber `DeleteTemplate()` loeschen und mit `ToggleFavorite()` als Favorit markieren. Der Dateiname wird aus dem Template-Namen abgeleitet (`sanitizeFilename`: Leerzeichen zu `_`, Sonderzeichen entfernt, Path-Traversal verhindert).

## Konsequenzen

### Positiv
- **Menschenlesbar**: JSON-Dateien koennen manuell editiert werden
- **Multi-Shell**: Ein Template deckt alle Shell-Typen ab; `ApplyTemplate` waehlt den richtigen Befehl basierend auf `terminalType`
- **Dynamische Variablen**: `TemplateContext` mit `CurrentDir`, `Username`, `Hostname` (erweiterbar um `SelectedText`, `Clipboard`)
- **Eingebettet + erweiterbar**: Standard-Templates im Binary, benutzerdefinierte im Dateisystem
- **CRUD-Operationen**: Vollstaendige Verwaltung ueber Frontend (Save, Update, Delete, Favorite)

### Negativ
- **Keine Validierung**: Ungueltige JSON-Dateien werden stillschweigend uebersprungen (`continue` in der Ladeschleife)
- **Keine Template-Versionierung**: Benutzer-Aenderungen ueberschreiben ohne Historie
- **Command-Injection-Risiko**: Template-Variablen werden ohne Escaping in Shell-Befehle eingesetzt
- **Dateiname-Kopplung**: Der Template-Name bestimmt den Dateinamen; Umbenennungen erfordern Loeschen und Neuanlage

## Alternativen

| Alternative | Grund fuer Ablehnung |
|---|---|
| **YAML** | Zusaetzliche Parser-Abhaengigkeit (`gopkg.in/yaml.v3`), kein Vorteil gegenueber JSON fuer diese einfache Struktur |
| **TOML** | Weniger verbreitet fuer Datenstrukturen mit verschachtelten Maps |
| **SQLite-Datenbank** | Overkill fuer ~10-50 Templates, verliert die Editierbarkeit einzelner Dateien |
| **Go-Code (Plugin-System)** | Zu komplex, Sicherheitsrisiko durch ausfuehrbaren Code, keine Hot-Reload-Moeglichkeit |
| **Einzelne JSON-Datei fuer alle Templates** | Merge-Konflikte, keine unabhaengige Verwaltung einzelner Templates |

## Betroffene Dateien / Module

- `templates/*.json` -- Template-Dateien (z.B. `list_files.json`), werden per `//go:embed templates` eingebettet
- `template/template.go` -- `Template`-Struct, `Manager` mit `LoadTemplates()`, `ApplyTemplate()`, `SaveTemplate()`, `UpdateTemplate()`, `DeleteTemplate()`, `ToggleFavorite()`, `sanitizeFilename()`
- `template/template_test.go` -- Tests mit `fstest.MapFS` fuer In-Memory-Template-Dateien
- `main.go` -- `//go:embed templates` und Uebergabe an `NewApp()`
- `app.go` -- `getTemplateContext()` baut `TemplateContext` mit OS-Informationen, delegiert CRUD-Operationen an `TemplateManager`
