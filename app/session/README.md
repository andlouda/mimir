# session

## Zweck
Persistierung und Wiederherstellung von Terminal-Sitzungen. Speichert den Zustand aller aktiven Terminals beim Schließen der Anwendung und lädt ihn beim nächsten Start.

## Inhalt
- `session.go` - Session-Datenmodell, Save/Load-Logik
- `session_test.go` - Unit Tests (75% Coverage)

## Verantwortlichkeiten
- Serialisierung/Deserialisierung von `SessionData` (JSON)
- Ermittlung des OS-spezifischen Konfigurationsverzeichnisses
- Erstellen des Konfigurationsverzeichnisses falls nicht vorhanden
- Sichere Dateiberechtigungen (0600 / 0700)

**Nicht hierhin gehört:**
- Terminal-Lifecycle-Management (siehe `terminal/`)
- UI-State-Management (siehe Frontend)

## Abhängigkeiten
- Keine externen Abhängigkeiten (nur Go Standardbibliothek)
- Wird von `app.go` verwendet

## Wichtige Hinweise
- Session-Datei: `{UserConfigDir}/mimir/mimir_session.json`
  - Windows: `%APPDATA%\mimir\mimir_session.json`
  - Linux: `~/.config/mimir/mimir_session.json`
  - macOS: `~/Library/Application Support/mimir/mimir_session.json`
- Dateiberechtigungen: 0600 (nur Owner lesen/schreiben)
- Verzeichnisberechtigungen: 0700
- Bei fehlender Session-Datei wird eine leere `SessionData` zurückgegeben (kein Fehler)
- Bei korrupter JSON-Datei wird ein Fehler zurückgegeben

## Beispiele

```go
// Session speichern
data := session.SessionData{
    Terminals: []session.TerminalState{
        {Type: "cmd", Name: "Terminal 1", Minimized: false},
    },
}
err := session.SaveSession(data)

// Session laden
loaded, err := session.LoadSession()
```
