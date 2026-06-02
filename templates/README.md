# templates

## Zweck
Vorgefertigte Befehlsvorlagen als JSON-Dateien. Diese werden zur Build-Zeit via `go:embed` in die Anwendung eingebettet und stehen sofort nach dem Start zur Verfügung.

## Inhalt
Jede JSON-Datei definiert ein Template mit Befehlen für verschiedene Shell-Typen:

| Datei | Beschreibung | Favorit |
|-------|-------------|---------|
| `list_files.json` | Dateien auflisten (ls/dir) | Nein |
| `ping_google.json` | Netzwerk-Konnektivität prüfen | Ja |
| `ress.json` | System-Ressourcen überwachen (htop/wmic) | Ja |
| `cd_home.json` | Ins Home-Verzeichnis wechseln | - |
| `show_ip.json` | IP-Adresse anzeigen | - |
| `hide_path.json` | Terminal-Prompt kürzen | - |
| u.a. | Weitere Netzwerk- und System-Templates | - |

## Verantwortlichkeiten
- Speicherort für JSON-Template-Definitionen
- Wird von `go:embed` in `main.go` eingebettet
- Wird auch zur Laufzeit von `template.Manager` für CRUD-Operationen verwendet

**Nicht hierhin gehört:**
- Template-Logik (siehe `template/`)
- Benutzerspezifische Templates (Annahme: zukünftig in UserConfigDir)

## Abhängigkeiten
- Eingebettet in die Binary via `main.go` (`//go:embed templates`)
- Geladen von `template.Manager.LoadTemplates()`

## Wichtige Hinweise
- **Dateiformat:** JSON mit Feldern `name`, `description`, `commands`, `favorite`
- **Commands-Map:** Keys sind Terminal-Typen (`bash`, `cmd`, `powershell`, `wsl`, `zsh`)
- **Go Template-Syntax:** Befehle können Go-Template-Variablen verwenden (z.B. `{{.CurrentDir}}`)
- **Laufzeit-Änderungen:** Save/Update/Delete schreiben auf das Dateisystem (`./templates/`), nicht in die eingebettete FS. Nach einem Rebuild werden Laufzeit-Änderungen in die Binary aufgenommen.
- **Sanitierung:** Dateinamen werden durch `sanitizeFilename()` gefiltert

## Beispiele

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
