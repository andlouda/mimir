# ADR-0005: Session-Persistierung als JSON im OS-Konfigurationsverzeichnis

## Status

Angenommen

## Kontext

Mimir verwaltet mehrere Terminal-Sitzungen gleichzeitig. Wenn der Benutzer die Anwendung schliesst und wieder oeffnet, sollen die vorherigen Terminal-Tabs wiederhergestellt werden -- nicht der Terminal-Inhalt selbst, sondern die Metadaten: welcher Shell-Typ war geoeffnet, welchen Namen hatte der Tab, und war er minimiert.

Die Session-Daten muessen:
- Beim Schliessen der Anwendung gespeichert werden
- Beim naechsten Start geladen werden
- Betriebssystem-uebergreifend im richtigen Konfigurationsverzeichnis abgelegt werden
- Mit restriktiven Dateiberechtigungen geschuetzt sein

## Entscheidung

Session-Daten werden als JSON-Datei unter `{os.UserConfigDir()}/mimir/mimir_session.json` gespeichert. Die Funktion `os.UserConfigDir()` liefert plattformabhaengig:
- Windows: `%AppData%` (z.B. `C:\Users\<user>\AppData\Roaming`)
- Linux: `$XDG_CONFIG_HOME` oder `~/.config`
- macOS: `~/Library/Application Support`

Die Datenstruktur in `session/session.go`:

```go
type TerminalState struct {
    Type      string `json:"type"`       // "cmd", "powershell", "wsl", "bash", "zsh"
    Name      string `json:"name"`       // Benutzerdefinierter Tab-Name
    Minimized bool   `json:"minimized"`  // Tab war minimiert
}

type SessionData struct {
    Terminals []TerminalState `json:"terminals"`
}
```

`SaveSession()` schreibt mit `os.WriteFile(filePath, jsonData, 0600)` -- Dateiberechtigung `0600` (nur Eigentuemer darf lesen/schreiben). Das Konfigurationsverzeichnis wird mit `os.MkdirAll(appConfigDir, 0700)` erstellt.

`LoadSession()` gibt bei fehlender Datei ein leeres `SessionData{}` ohne Fehler zurueck (`os.IsNotExist` wird abgefangen). Bei korruptem JSON wird ein Fehler zurueckgegeben.

In `main.go` wird der `OnBeforeClose`-Hook genutzt, um `app.SaveCurrentSession()` vor dem Schliessen aufzurufen. In `app.go` sammelt `SaveCurrentSession()` alle Eintraege aus `activeTerminalStates` (geschuetzt durch `stateMu`).

Die Terminal-States werden vom Frontend ueber `UpdateTerminalState(id, type, name, minimized)` laufend aktualisiert und ueber `RemoveTerminalState(id)` beim Schliessen eines Tabs entfernt.

## Konsequenzen

### Positiv
- **Plattformunabhaengiger Speicherort**: `os.UserConfigDir()` waehlt das richtige Verzeichnis fuer jedes OS
- **Restriktive Berechtigungen**: `0600` fuer die Datei, `0700` fuer das Verzeichnis -- keine Lesbarkeit fuer andere Benutzer
- **Graceful Degradation**: Fehlende Session-Datei fuehrt nicht zu einem Fehler, sondern zu einem leeren Startzustand
- **Einfache Struktur**: Flache JSON-Datei ohne Schema-Versionierung -- bei wenigen Feldern vertretbar
- **Automatisch**: Speicherung erfolgt transparent beim Schliessen, ohne Benutzerinteraktion

### Negativ
- **Keine Terminal-Inhalte**: Nur Metadaten werden gespeichert; der Terminal-Scrollback geht verloren
- **Kein Crash-Schutz**: Bei einem Absturz (ohne `OnBeforeClose`) gehen die Session-Daten verloren
- **Keine Schema-Migration**: Wenn `TerminalState` um Felder erweitert wird, koennen alte Session-Dateien unvollstaendige Daten liefern
- **Einzelbenutzer**: Keine Unterstuetzung fuer mehrere Mimir-Instanzen oder Profile

## Alternativen

| Alternative | Grund fuer Ablehnung |
|---|---|
| **SQLite** | Overkill fuer eine einzelne Datei mit ~1 KB; zusaetzliche CGO-Abhaengigkeit |
| **Windows Registry / macOS Defaults** | Nicht plattformuebergreifend, komplexere API |
| **Automatisches Speichern alle N Sekunden** | Wuerde Dateisystem-I/O unter Last erzeugen; `OnBeforeClose` ist fuer den Anwendungsfall ausreichend |
| **BoltDB / BadgerDB** | Key-Value-Stores sind ueberdimensioniert fuer eine einzige Session-Datei |
| **Klartext (CSV, INI)** | Weniger strukturiert als JSON, keine verschachtelten Typen moeglich |

## Betroffene Dateien / Module

- `session/session.go` -- `TerminalState`, `SessionData`, `getSessionFilePath()`, `SaveSession()`, `LoadSession()`
- `session/session_test.go` -- Tests fuer Save/Load-Roundtrip, fehlende Datei, korruptes JSON, leere Session; sichert/restauriert Original-Datei in Tests
- `app.go` -- `activeTerminalStates`-Map, `SaveCurrentSession()`, `UpdateTerminalState()`, `RemoveTerminalState()`, `GetLoadedSessionData()`
- `main.go` -- `OnBeforeClose`-Hook ruft `app.SaveCurrentSession()` auf
