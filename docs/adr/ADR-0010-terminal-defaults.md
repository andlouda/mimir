# ADR-0010: Terminal Defaults

## Status

Vorgeschlagen

## Kontext

Neue Terminals in Mimir verwenden aktuell hardcodierte Werte fuer Shell, Schriftart, Schriftgroesse, Cursor-Stil, Scrollback-Laenge und Theme. Diese Werte sind direkt in `createTerminalInstance` (App.svelte) eingebettet:

```js
const terminal = new Terminal({
  cursorBlink: true,
  cursorStyle: 'bar',
  fontFamily: "'JetBrains Mono', 'Fira Code', 'Cascadia Code', monospace",
  fontSize: 13,
  lineHeight: 1.35,
  // ...
  theme: { background: '#0c0e14', foreground: '#c9d1d9', cursor: '#63b3ed', ... }
});
```

Es gibt keine Moeglichkeit, diese Werte nutzerseitig zu konfigurieren, ohne den Quellcode zu aendern. Der Default-Terminal-Typ wird bereits in `localStorage` gespeichert (`mimir-default-terminal-type`), aber alle visuellen und verhaltensbezogenen Defaults fehlen.

## Entscheidung

Es wird ein persistiertes **Terminal Defaults**-System eingefuehrt mit folgenden Einstellungen:

| Einstellung     | Typ      | Default-Wert                                                     |
|-----------------|----------|------------------------------------------------------------------|
| `defaultShell`  | `string` | Plattform-abhaengig (`bash`/`wsl`/`cmd`)                        |
| `fontSize`      | `int`    | `13`                                                             |
| `fontFamily`    | `string` | `'JetBrains Mono', 'Fira Code', 'Cascadia Code', monospace`     |
| `cursorStyle`   | `string` | `bar`                                                            |
| `cursorBlink`   | `bool`   | `true`                                                           |
| `scrollback`    | `int`    | `5000`                                                           |
| `themeName`     | `string` | `mimir-dark`                                                     |
| `lineHeight`    | `float`  | `1.35`                                                           |

### Speicher

Datei: `~/.config/mimir/terminal_defaults.json`

Pattern: identisch zu `ai_settings.json` — einfacher JSON-Blob, Load/Save ueber `safeio.AtomicWriteFile`.

### Backend

Neue Datei `terminal_defaults.go` im `main`-Package:

```go
type TerminalDefaults struct {
    DefaultShell string  `json:"defaultShell"`
    FontSize     int     `json:"fontSize"`
    FontFamily   string  `json:"fontFamily"`
    CursorStyle  string  `json:"cursorStyle"`
    CursorBlink  bool    `json:"cursorBlink"`
    Scrollback   int     `json:"scrollback"`
    ThemeName    string  `json:"themeName"`
    LineHeight   float64 `json:"lineHeight"`
}
```

Zwei Wails-Methoden:

- `GetTerminalDefaults() TerminalDefaults`
- `SaveTerminalDefaults(json string) error`

### Frontend

- Settings-Card "Terminal Defaults" mit Formular fuer alle Einstellungen
- `createTerminalInstance` liest die gespeicherten Defaults statt Hardcodes
- Aenderungen gelten nur fuer **neue** Terminals — kein Live-Update bestehender Terminals

### Theme

Vorerst wird nur ein einzelnes Theme `mimir-dark` unterstuetzt. Die Theme-Palette wird als Frontend-Dictionary definiert und ist spaeter erweiterbar (z.B. `solarized`, `monokai`). Das `themeName`-Feld in den Defaults dient als Lookup-Key.

## Konsequenzen

### Positiv

- **Nutzerkonfigurierbar**: Schrift, Groesse, Cursor und Theme koennen ohne Code-Aenderung angepasst werden
- **Persistiert**: Einstellungen bleiben ueber Neustarts erhalten
- **Einfach**: minimale Aenderungen, kein neues Package noetig
- **Erweiterbar**: neue Defaults oder Themes koennen spaeter additiv hinzugefuegt werden

### Negativ

- **Kein Live-Update**: bestehende Terminals behalten ihre Einstellungen bis zum Schliessen
- **Kein Per-Terminal-Override**: alle neuen Terminals bekommen dieselben Defaults
- **Nur ein Theme**: erweiterte Theme-Auswahl erfordert zusaetzliche Arbeit

## Alternativen

| Alternative | Grund fuer Ablehnung |
|---|---|
| Pro-Terminal-Settings | Zu komplex fuer den aktuellen Stand, kein User-Request |
| Live-Update bestehender Terminals | xterm.js `options`-Setter funktioniert, aber erfordert Re-Fit und Theme-Neuanwendung auf alle Instanzen — erhoehte Komplexitaet |
| Theme in separater Datei | Unnoetige Fragmentierung — ein JSON-Blob reicht |
| localStorage statt Datei | Nicht konsistent mit bestehenden Config-Patterns (`ai_settings.json`, `ssh_profiles.json`) |

## Betroffene Dateien

| Datei | Aenderung |
|-------|----------|
| `terminal_defaults.go` | Neues File: Struct, Load, Save, Wails-Methoden |
| `app.go` | Feld `terminalDefaults`, Init in `NewApp` |
| `frontend/wailsjs/go/main/App.js` | 2 neue Bindings |
| `frontend/src/App.svelte` | Settings-Card, `createTerminalInstance` liest Defaults |
