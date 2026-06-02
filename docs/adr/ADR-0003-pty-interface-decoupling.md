# ADR-0003: Writer-Interface zur Entkopplung vom PTY

## Status

Angenommen

## Kontext

Das Template-System in `template/template.go` muss Befehle an ein Terminal senden. Urspruenglich haette `ApplyTemplate` eine direkte Abhaengigkeit auf `*conpty.ConPty` haben koennen, was zu folgenden Problemen fuehrt:

1. **Testbarkeit**: `*conpty.ConPty` kann nur auf Windows instanziiert werden und benoetigt einen laufenden Shell-Prozess. Unit-Tests fuer das Template-System waeren auf Windows beschraenkt und haetten Seiteneffekte (tatsaechliche Prozesse starten).
2. **Kopplung**: Das `template`-Paket wuerde eine transitive Abhaengigkeit auf `github.com/UserExistsError/conpty` erhalten, obwohl es nur die `Write`-Methode benoetigt.
3. **Plattform-Portabilitaet**: Auf Nicht-Windows-Systemen koennte das Template-Paket nicht kompiliert oder getestet werden.

## Entscheidung

In `template/template.go` wird ein `Writer`-Interface definiert:

```go
type Writer interface {
    Write(p []byte) (n int, err error)
}
```

Die Methode `ApplyTemplate` akzeptiert dieses Interface als Parameter:

```go
func (m *Manager) ApplyTemplate(id int, templateName string, terminalType string, pty Writer, ctx TemplateContext) error
```

In `terminal/terminal.go` gibt `GetPty()` den Rueckgabetyp `io.Writer` zurueck (nicht `*conpty.ConPty`):

```go
func (m *Manager) GetPty(id int) (io.Writer, bool)
```

Da `*conpty.ConPty` bereits `io.Writer` implementiert, ist keine Adapter-Schicht noetig. In `app.go` wird der Rueckgabewert von `GetPty()` direkt an `ApplyTemplate()` uebergeben:

```go
p, ok := a.TerminalManager.GetPty(id)
a.TemplateManager.ApplyTemplate(id, templateName, terminalType, p, ctx)
```

Historischer Hinweis: Zum Zeitpunkt dieser ADR gab `terminal/terminal_unix.go` `nil, false` zurueck. Aktuell existiert eine native PTY-Implementierung fuer Linux/macOS ueber `github.com/creack/pty`.

## Konsequenzen

### Positiv
- **Testbar ohne OS-Abhaengigkeit**: In `template/template_test.go` wird ein `mockWriter` mit `bytes.Buffer` verwendet, um Template-Ausfuehrung vollstaendig in-memory zu testen
- **Plattformunabhaengige Tests**: `go test ./...` laeuft auf Windows, Linux und macOS mit den jeweiligen Build-Tag-Implementierungen
- **Saubere Paket-Grenzen**: `template`-Paket hat keine Abhaengigkeit auf `terminal`-Paket oder `conpty`-Library
- **Zukunftssicher**: Ein Unix-PTY (z.B. `creack/pty`) muesste nur `io.Writer` implementieren, um mit dem Template-System kompatibel zu sein

### Negativ
- **Indirektion**: Der Aufrufer (`app.go`) muss den Writer aus `GetPty()` holen und an `ApplyTemplate()` uebergeben, statt eines direkten Aufrufs
- **Interface-Granularitaet**: Das `Writer`-Interface in `template` ist identisch mit `io.Writer`, koennte aber zu Verwirrung fuehren, da es ein paket-eigener Typ ist

## Alternativen

| Alternative | Grund fuer Ablehnung |
|---|---|
| **Direkte `*conpty.ConPty`-Abhaengigkeit** | Keine Testbarkeit ohne Windows, enge Kopplung zwischen template und terminal |
| **`io.Writer` direkt als Parametertyp (ohne eigenes Interface)** | Funktional gleichwertig, aber ein eigenes Interface dokumentiert die Absicht explizit und erlaubt spaetere Erweiterung (z.B. `WriteString`) |
| **Template gibt Command-String zurueck, Aufrufer schreibt** | Wuerde die Template-Logik (Variablen-Substitution + Senden) aufsplitten und Code-Duplikation im Aufrufer erzeugen |

## Betroffene Dateien / Module

- `template/template.go` -- Definition des `Writer`-Interface, `ApplyTemplate()` akzeptiert `Writer` statt konkretem Typ
- `template/template_test.go` -- `mockWriter`-Struct mit `bytes.Buffer` fuer Unit-Tests
- `terminal/terminal.go` -- `GetPty()` gibt `io.Writer` zurueck statt `*conpty.ConPty`
- `terminal/terminal_unix.go` -- `GetPty()` gibt `nil, false` zurueck
- `app.go` -- `ApplyTemplate()` verbindet `GetPty()`-Ergebnis mit Template-Ausfuehrung
