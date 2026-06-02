# ADR-0001: Wails v2 als Hybrid-Desktop-Framework

## Status

Angenommen

## Kontext

Mimir ist ein Desktop-Terminal-Manager, der eine native Anwendung mit moderner Web-UI verbindet.

Anforderungen:

- Go-Backend fuer Terminal-, SSH-, Dateisystem-, Recording-, AI- und Update-Logik
- reaktive UI fuer mehrere Terminal-Sitzungen, Sidebar, Settings, SSH-Profile, Notes und Workflows
- native Desktop-Verpackung fuer Windows, Linux und macOS
- eingebettete Frontend-Assets
- direkte Go <-> Frontend Bindings

## Entscheidung

Mimir verwendet Wails v2.

Das Frontend verwendet inzwischen Svelte 5 und Vite 8. Die kompilierten Assets aus `frontend/dist` werden ueber `//go:embed all:frontend/dist` in die Go-Binary eingebettet.

Die `App`-Struct wird ueber Wails `Bind` exponiert. Oeffentliche Methoden auf `App` werden als JS/TS-Bindings unter `frontend/wailsjs/` generiert.

Terminal-Output wird nicht ueber normale Binding-Returns transportiert, sondern ueber Wails Events:

```text
terminal-output-{id}
terminal-closed-{id}
terminal-disconnected-{id}
```

## Konsequenzen

### Positiv

- Einzelne Desktop-App mit Go-Backend und WebView-Frontend
- Wails generiert Bindings fuer Go-Methoden
- Go eignet sich gut fuer Terminal-I/O, SSH, Files, SQLite und Nebenlaeufigkeit
- Frontend kann mit Svelte/xterm.js eine dichte Terminal-UI bauen
- Release-Artefakte koennen ueber Wails pro Plattform gebaut werden

### Negativ

- Wails-spezifische Runtime-APIs sind Teil der App-Schicht
- WebView2/WebKit-Systemabhaengigkeiten muessen auf Zielsystemen vorhanden sein
- Cross-platform Release-Builds brauchen plattformspezifische Umgebungen
- `frontend/wailsjs/` ist generiert und muss bei Binding-Aenderungen aktuell bleiben

## Alternativen

| Alternative | Grund fuer Ablehnung |
|-------------|----------------------|
| Electron | Zu schwergewichtig; Backend waere Node-basiert oder muesste als Sidecar laufen. |
| Tauri | Rust-zentrierter Stack, waehrend das Backend bewusst in Go liegt. |
| Fyne/Gio | Kein xterm.js/Web-Frontend; Terminal-UI waere deutlich aufwaendiger. |
| TUI | Nicht passend fuer File Browser, Split Pane UI, Notes und Rich Settings. |

## Betroffene Dateien

- `main.go`
- `app.go`
- `app_*.go`
- `wails.json`
- `frontend/`
- `frontend/wailsjs/`
- `go.mod`
