# ADR-0006: sync.Mutex fuer die activeTerminalStates-Map

## Status

Angenommen

## Kontext

In `app.go` verwaltet der `App`-Struct eine `activeTerminalStates map[int]session.TerminalState`. Diese Map wird von mehreren Goroutinen gleichzeitig geschrieben und gelesen:

1. **Frontend-Aufrufe** (Wails-Binding-Goroutinen): `UpdateTerminalState()` und `RemoveTerminalState()` werden vom Frontend bei Tab-Aenderungen aufgerufen. Wails fuehrt gebundene Methoden in separaten Goroutinen aus.
2. **Session-Speicherung** (`SaveCurrentSession()`): Liest alle Eintraege aus der Map, wird ueber den `OnBeforeClose`-Hook aufgerufen.
3. **Potenzielle Parallelitaet**: Mehrere Terminals koennen gleichzeitig umbenannt, minimiert oder geschlossen werden, waehrend die Session gespeichert wird.

Go-Maps sind nicht thread-sicher. Gleichzeitiges Lesen und Schreiben fuehrt zu einer `concurrent map read and map write`-Panic zur Laufzeit. Ohne Synchronisation wuerde ein Data Race entstehen, der vom Go Race Detector (`-race` Flag) erkannt wird.

## Entscheidung

Ein `sync.Mutex` namens `stateMu` wird zum `App`-Struct hinzugefuegt:

```go
type App struct {
    // ...
    activeTerminalStates map[int]session.TerminalState
    stateMu              sync.Mutex
}
```

Alle Zugriffe auf `activeTerminalStates` werden durch `stateMu` geschuetzt:

- `UpdateTerminalState()`: `a.stateMu.Lock()` / `defer a.stateMu.Unlock()` vor dem Map-Write
- `RemoveTerminalState()`: `a.stateMu.Lock()` / `defer a.stateMu.Unlock()` vor `delete()`
- `SaveCurrentSession()`: `a.stateMu.Lock()` zum Kopieren der Map-Eintraege in ein Slice, dann `a.stateMu.Unlock()` vor dem eigentlichen I/O (JSON-Marshalling und Dateischreiben)

In `SaveCurrentSession()` wird der Lock bewusst nur fuer die Map-Iteration gehalten und vor dem I/O freigegeben, um die Lock-Dauer zu minimieren:

```go
a.stateMu.Lock()
var terminalsToSave []session.TerminalState
for _, state := range a.activeTerminalStates {
    terminalsToSave = append(terminalsToSave, state)
}
a.stateMu.Unlock()
// I/O ohne Lock
```

## Konsequenzen

### Positiv
- **Keine Data Races**: Gleichzeitige Map-Zugriffe aus verschiedenen Goroutinen sind sicher
- **Keine Panics**: Die `concurrent map read and map write`-Runtime-Panic wird verhindert
- **Minimale Lock-Dauer**: In `SaveCurrentSession` wird der Lock nur fuer die Map-Kopie gehalten, nicht fuer die Datei-I/O
- **Einfach und idiomatisch**: `sync.Mutex` ist der Standard-Mechanismus in Go fuer Map-Schutz

### Negativ
- **Serielle Zugriffe**: Zwei gleichzeitige `UpdateTerminalState`-Aufrufe werden serialisiert (in der Praxis vernachlaessigbar, da die kritische Sektion nur eine Map-Zuweisung ist)
- **Kein RWMutex**: Ein `sync.RWMutex` wuerde parallele Lesezugriffe erlauben, ist aber bei dieser Zugriffsfrequenz (sporadische Frontend-Updates) nicht noetig

## Alternativen

| Alternative | Grund fuer Ablehnung |
|---|---|
| **`sync.Map`** | Optimiert fuer hohe Lese-/niedrige Schreibraten mit vielen Goroutinen; hier ueberflussig, da Zugriffe selten sind und `sync.Map` keinen typisierten Zugriff bietet |
| **`sync.RWMutex`** | Wuerde parallele Reads erlauben, aber die Map wird selten gelesen (nur bei `SaveCurrentSession`); der Overhead einer RW-Lock-Semantik lohnt sich nicht |
| **Channel-basierte Serialisierung** | Deutlich komplexer (eigene Goroutine mit Select-Loop), fuer eine einfache Map-Synchronisation ueberdimensioniert |
| **Kein Schutz (Map nur im Haupt-Goroutine)** | Nicht moeglich, da Wails Binding-Methoden in separaten Goroutinen ausfuehrt |

## Betroffene Dateien / Module

- `app.go` -- `stateMu sync.Mutex` im `App`-Struct; Lock/Unlock in `UpdateTerminalState()`, `RemoveTerminalState()`, `SaveCurrentSession()`
