# ADR-0007: fs.FS-Interface statt embed.FS fuer Template-Manager

## Status

Angenommen

## Kontext

In `main.go` werden die Templates ueber `//go:embed templates` als `embed.FS` in die Binary eingebettet und an `NewApp(embeddedTemplates)` uebergeben. Der `template.Manager` muss diese Templates lesen.

Urspruenglich akzeptierte `NewApp()` den konkreten Typ `embed.FS`. Das Problem:

1. **Testbarkeit**: `embed.FS` kann nur durch den Go-Compiler mittels `//go:embed`-Direktive erzeugt werden. In Unit-Tests ist es nicht moeglich, einen `embed.FS` mit beliebigem Inhalt zu erstellen.
2. **Test-Isolation**: Um `LoadTemplates()` zu testen, muessten die tatsaechlichen Template-Dateien im `templates/`-Verzeichnis vorhanden sein -- Tests waeren von Dateisystem-Inhalten abhaengig.
3. **Flexibilitaet**: Spaetere Erweiterungen (z.B. Templates aus einem entfernten Speicher laden) waeren ohne Interface-Aenderung nicht moeglich.

## Entscheidung

Der `template.Manager` akzeptiert `fs.FS` (aus dem Standardpaket `io/fs`) statt `embed.FS`:

```go
type Manager struct {
    templates         []Template
    embeddedTemplates fs.FS
}

func NewManager(embeddedTemplates fs.FS) *Manager {
    return &Manager{
        embeddedTemplates: embeddedTemplates,
    }
}
```

In `main.go` wird weiterhin `embed.FS` uebergeben -- da `embed.FS` das `fs.FS`-Interface implementiert, ist keine Anpassung noetig:

```go
//go:embed templates
var templates embed.FS

app := NewApp(templates) // embed.FS implementiert fs.FS
```

In `app.go` akzeptiert `NewApp()` ebenfalls `embed.FS`, da dies der oeffentliche Einstiegspunkt ist. Die Konvertierung erfolgt implizit durch das Go-Typsystem.

In `template/template_test.go` wird `fstest.MapFS` (aus `testing/fstest`) als In-Memory-Dateisystem verwendet:

```go
func createTestFS() fs.FS {
    return fstest.MapFS{
        "templates/test_list.json": &fstest.MapFile{
            Data: []byte(`{"name": "List Files", ...}`),
        },
        "templates/test_cd.json": &fstest.MapFile{
            Data: []byte(`{"name": "Change Dir", ...}`),
        },
    }
}

func createTestManager() *Manager {
    m := NewManager(createTestFS())
    m.LoadTemplates()
    return m
}
```

`LoadTemplates()` verwendet `fs.ReadDir()` und `fs.ReadFile()` (nicht `os.ReadDir` oder `embed.FS`-spezifische Methoden), sodass jede `fs.FS`-Implementierung funktioniert.

## Konsequenzen

### Positiv
- **Vollstaendige Test-Isolation**: Tests erstellen Templates als In-Memory-Daten, keine Dateisystem-Abhaengigkeit
- **Deterministische Tests**: Template-Inhalte sind im Testcode definiert, nicht in externen Dateien
- **Keine Code-Aenderung in main.go**: `embed.FS` implementiert `fs.FS` automatisch
- **Erweiterbar**: Spaeter koennten Templates aus ZIP-Archiven (`zip.Reader` implementiert `fs.FS`), HTTP-Responses oder anderen Quellen geladen werden
- **Standard-Library**: Keine externen Abhaengigkeiten -- `io/fs`, `testing/fstest` sind Teil der Go-Standardbibliothek

### Negativ
- **Nur Lese-Interface**: `fs.FS` unterstuetzt kein Schreiben. `SaveTemplate()`, `UpdateTemplate()` und `DeleteTemplate()` nutzen weiterhin `os.WriteFile` und `os.Remove` mit Dateisystem-Pfaden (`./templates/`), nicht das `fs.FS`-Interface
- **Zwei Quellen**: Eingebettete Templates kommen aus `fs.FS`, benutzerdefinierte werden direkt im Dateisystem gespeichert und nach Reload wieder ueber `fs.FS` gelesen

## Alternativen

| Alternative | Grund fuer Ablehnung |
|---|---|
| **`embed.FS` als konkreter Typ beibehalten** | Keine Unit-Tests ohne reales Dateisystem und `//go:embed`-Direktive moeglich |
| **Eigenes `TemplateStore`-Interface (ReadDir + ReadFile)** | Ueberfluessig, da `fs.FS` genau dieses Interface bereits definiert |
| **`afero.FS` (spf13/afero)** | Externe Abhaengigkeit; `io/fs` + `testing/fstest` aus der Standardbibliothek reichen fuer den Anwendungsfall |
| **Templates als Go-Konstanten in Tests hardcoden** | Wuerde `LoadTemplates()` nicht testen, sondern umgehen |

## Betroffene Dateien / Module

- `template/template.go` -- `Manager.embeddedTemplates` hat Typ `fs.FS` statt `embed.FS`; `NewManager()` akzeptiert `fs.FS`; `LoadTemplates()` nutzt `fs.ReadDir()` und `fs.ReadFile()`
- `template/template_test.go` -- `createTestFS()` gibt `fstest.MapFS` zurueck; `createTestManager()` erzeugt Manager mit In-Memory-Dateisystem
- `app.go` -- `NewApp()` akzeptiert `embed.FS`, die implizit als `fs.FS` an `template.NewManager()` weitergegeben wird
- `main.go` -- Keine Aenderung noetig, `embed.FS` implementiert `fs.FS`
