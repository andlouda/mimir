# ADR-0008: Unit-Test-Strategie ohne externe Abhaengigkeiten

## Status

Angenommen

## Kontext

Mimir benoetigt eine Test-Strategie, die:
- Plattformuebergreifend funktioniert (Tests muessen auf Linux laufen, auch wenn ConPTY nur unter Windows verfuegbar ist)
- Keine externen Test-Frameworks oder Mocking-Libraries erfordert
- Die Kernlogik (Template-Ausfuehrung, Session-Persistierung, Pfad-Validierung) abdeckt, ohne tatsaechliche Terminal-Prozesse zu starten
- In CI/CD-Pipelines ohne spezielle Infrastruktur lauffaehig ist

Die drei testbaren Module sind:
1. `template/` -- Template-Laden, Variablen-Substitution, Befehlsausgabe, Dateinamen-Sanitisierung
2. `session/` -- Session speichern/laden, Fehlerbehandlung bei fehlenden/korrupten Dateien
3. `app.go` -- Pfad-Validierung (`isValidPath`)

Das `terminal/`-Paket ist nicht unit-testbar, da es direkt `conpty.Start()` und `wailsruntime.EventsEmit()` aufruft.

## Entscheidung

### Test-Framework

Ausschliesslich Go-Standard-`testing`-Paket. Keine externen Abhaengigkeiten wie `testify`, `gomock` oder `ginkgo`. Assertions erfolgen ueber manuelle `if`-Checks mit `t.Errorf()` / `t.Fatalf()`.

### Testmuster

**Table-Driven Tests** werden fuer parametrisierbare Testfaelle verwendet:

```go
// app_test.go
func TestIsValidPath(t *testing.T) {
    tests := []struct {
        name  string
        path  string
        valid bool
    }{
        {"empty path", "", false},
        {"path traversal with ..", "../etc/passwd", false},
        {"absolute linux path", "/home/user/documents", true},
        // ...
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := isValidPath(tt.path)
            if result != tt.valid {
                t.Errorf("isValidPath(%q) = %v, want %v", tt.path, result, tt.valid)
            }
        })
    }
}
```

```go
// template/template_test.go - sanitizeFilename
tests := []struct {
    input    string
    expected string
}{
    {"simple", "simple"},
    {"with spaces", "with_spaces"},
    {"../../../etc/passwd", "etcpasswd"},
    // ...
}
```

**Mock Writer** fuer PTY-Entkopplung (siehe ADR-0003):

```go
// template/template_test.go
type mockWriter struct {
    buf bytes.Buffer
}
func (m *mockWriter) Write(p []byte) (int, error) {
    return m.buf.Write(p)
}
```

Der `mockWriter` erfasst alle an den PTY geschriebenen Bytes in einem `bytes.Buffer`. Tests pruefen den geschriebenen Inhalt direkt:

```go
w := &mockWriter{}
m.ApplyTemplate(1, "List Files", "bash", w, TemplateContext{})
if w.buf.String() != "ls -la\r\n" { ... }
```

**In-Memory-Dateisystem** fuer Template-Tests (siehe ADR-0007):

```go
func createTestFS() fs.FS {
    return fstest.MapFS{
        "templates/test_list.json": &fstest.MapFile{Data: []byte(`{...}`)},
    }
}
```

**Dateisystem-Isolation** fuer Session-Tests:

```go
// session/session_test.go
original, originalExists := readOriginal(filePath)
defer restoreOriginal(filePath, original, originalExists)
```

Session-Tests sichern die vorhandene Session-Datei vor dem Test und stellen sie per `defer` wieder her. So bleiben echte Benutzerdaten bei lokalen Testlaeufen erhalten.

### Abgedeckte Testfaelle

**template/template_test.go**:
- `TestLoadTemplates` -- Laden aus `fstest.MapFS`, korrekte Anzahl
- `TestReloadTemplates` -- Neulade-Verhalten
- `TestApplyTemplate` -- Befehlsausgabe fuer bekanntes Template ("ls -la\r\n")
- `TestApplyTemplateWithVariables` -- Go-Template-Substitution (`{{.CurrentDir}}` -> `/home/user`)
- `TestApplyTemplateNotFound` -- Fehler bei unbekanntem Template-Namen
- `TestApplyTemplateUnsupportedType` -- Fehler bei nicht unterstuetztem Shell-Typ
- `TestSanitizeFilename` -- Table-driven Tests fuer Sonderzeichen, Path-Traversal, Leerzeichen

**session/session_test.go**:
- `TestSaveAndLoadSession` -- Roundtrip: Speichern, Datei pruefen, Laden, Felder vergleichen; Dateiberechtigung `0600` pruefen
- `TestLoadSessionFileNotExists` -- Leeres Ergebnis ohne Fehler bei fehlender Datei
- `TestLoadSessionCorruptedFile` -- Fehler bei ungueltigem JSON
- `TestSaveEmptySession` -- Leere Session speichern und laden

**app_test.go**:
- `TestIsValidPath` -- Table-driven: leere Pfade, relative Pfade, Path-Traversal (`..`), absolute Pfade (Linux und Windows)

## Konsequenzen

### Positiv
- **Keine externen Abhaengigkeiten**: `go.mod` enthaelt keine Test-Libraries; `go test ./...` funktioniert sofort nach `go mod download`
- **Plattformuebergreifend**: Template-Tests und App-Tests laufen auf Linux, Windows und macOS ohne ConPTY
- **Schnelle Ausfuehrung**: Alle Tests laufen in Millisekunden, kein Netzwerk, keine Prozesse
- **Reproduzierbar**: `fstest.MapFS` und `mockWriter` eliminieren externe Abhaengigkeiten
- **Idiomatisch**: Table-driven Tests und Subtests (`t.Run`) sind Go-Standard

### Negativ
- **Keine Terminal-Integration-Tests**: Das `terminal/`-Paket hat keine Tests; ConPTY-Interaktion wird nicht automatisiert getestet
- **Keine Assertion-Helpers**: Wiederholte `if result != expected { t.Errorf(...) }`-Bloecke statt `assert.Equal()`
- **Session-Tests auf echtem Dateisystem**: `session_test.go` schreibt in `os.UserConfigDir()`, nicht in ein temporaeres Verzeichnis
- **Kein Coverage-Ziel**: Keine definierte Mindest-Testabdeckung

## Alternativen

| Alternative | Grund fuer Ablehnung |
|---|---|
| **testify/assert + testify/mock** | Externe Abhaengigkeit; fuer die aktuelle Testgroesse (3 Dateien, ~200 Zeilen) nicht gerechtfertigt |
| **gomock (Code-generierte Mocks)** | Zu schwergewichtig; der handgeschriebene `mockWriter` ist 4 Zeilen lang |
| **ginkgo/gomega (BDD-Framework)** | Anderer Teststil, der nicht zum restlichen Go-Code passt; zusaetzliche Lernkurve |
| **Integration-Tests mit echtem ConPTY** | Erfordern Windows-Umgebung, laufende Shell-Prozesse, und sind nicht-deterministisch (Timing-abhaengig) |
| **Docker-basierte Tests** | Overkill fuer den aktuellen Umfang; ConPTY ist in Containern nicht verfuegbar |

## Betroffene Dateien / Module

- `template/template_test.go` -- `mockWriter`, `createTestFS()`, `createTestManager()`, Tests fuer Load/Reload/Apply/Sanitize
- `session/session_test.go` -- Save/Load-Roundtrip, fehlende Datei, korruptes JSON, leere Session, Helper-Funktionen `readOriginal()`/`restoreOriginal()`
- `app_test.go` -- Table-driven Test fuer `isValidPath()` mit 10 Testfaellen
