# Transcript Module

Status: Implementiert (v0.2.5)

Das Transcript-Modul speichert die komplette Byte-für-Byte-Ausgabe jedes
Terminals dauerhaft auf Platte und macht sie über einen Viewer im
Terminal-Header (📜-Button) wieder zugänglich — auch nach Reboot, Crash oder
nachdem das Terminal längst geschlossen wurde.

Der Use-Case ist nicht "Session-Restore" im Sinn eines Reboot-Workspace
(siehe [`reboot-workspace-restore.md`](reboot-workspace-restore.md)), sondern
das pragmatische "ich war gestern bei dem Bug fast dran — was war noch mal
in dem Terminal?".

---

## 1. Architektur im Überblick

```
┌──────────────────────────────────────────────────────────────────┐
│  Frontend                                                        │
│                                                                  │
│  SplitPane.svelte    ──opentranscript──>  App.svelte             │
│  ( 📜 Header-Button)                       (openTranscriptViewer)│
│                                              │                   │
│                                              ▼                   │
│                                       TranscriptViewerModal      │
│                                              │                   │
│                                              ▼                   │
│                         lib/transcript/transcriptApi.js          │
│                         lib/transcript/cleanTranscript.js        │
└──────────────────────────────────────────────────────────────────┘
                                       │  Wails IPC
                                       ▼
┌──────────────────────────────────────────────────────────────────┐
│  Backend (Go)                                                    │
│                                                                  │
│  app.go                                                          │
│     ListTranscripts()           ───┐                             │
│     GetTerminalTranscriptFull() ───┼──> transcript pkg           │
│     AppendTerminalTranscript()  ───┤                             │
│     SaveTranscriptMetadata()    ───┘                             │
│                                                                  │
│  transcript/                                                     │
│     Append / ReadTail / ReadFull                                 │
│     WriteMetadata / ReadMetadata                                 │
│     List                                                         │
└──────────────────────────────────────────────────────────────────┘
                                       │
                                       ▼
┌──────────────────────────────────────────────────────────────────┐
│  Disk                                                            │
│  ~/.config/mimir/transcripts/                                    │
│     <resumeID>.log         ← Append-only Roh-Stream              │
│     <resumeID>.json        ← Side-Car: Name, Type, SSH-Profil    │
└──────────────────────────────────────────────────────────────────┘
```

---

## 2. Identifikation: `resumeID`

Jedes Terminal hat eine UUID, die im Frontend per `crypto.randomUUID()`
beim Anlegen erzeugt wird (siehe `frontend/src/lib/util.js`
`generateResumeId`). Diese ID:

- ist über die gesamte Lebensdauer des Terminals stabil
- überlebt App-Restart (steht in `session.json`)
- ist der Schlüssel für sowohl `.log` als auch `.json` auf Platte
- wird im Backend gegen `^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$` validiert,
  bevor Pfade gebaut werden — Path-Traversal über bösartige IDs ist damit
  ausgeschlossen

---

## 3. Speicherort und Layout

Standard-Verzeichnis: `os.UserConfigDir() / mimir / transcripts /`

Auf den jeweiligen Plattformen heißt das:

| OS      | Pfad                                                         |
|---------|--------------------------------------------------------------|
| Linux   | `~/.config/mimir/transcripts/`                               |
| macOS   | `~/Library/Application Support/mimir/transcripts/`           |
| Windows | `%AppData%\mimir\transcripts\`                               |

Pro Terminal entstehen bis zu zwei Dateien:

```
<resumeID>.log    Rohe Terminal-Ausgabe, append-only, Permissions 0600
<resumeID>.json   Side-Car-Metadaten (Name, Typ, SSH-Profil), Perms 0600
```

Existenz der `.json`-Datei ist optional — fehlt sie, ist der Eintrag in
der UI ein „Geschlossenes Terminal" mit Größe + Datum, sonst nichts.
Die `.log` ist die Quelle der Wahrheit für den Inhalt; die `.json` ist
nur Label.

---

## 4. Backend-API (`mimir/transcript`)

Reines Go-Paket ohne Wails-Abhängigkeiten. Vollständig testbar.

### 4.1 Datentypen

```go
type Metadata struct {
    Name         string    `json:"name,omitempty"`
    Type         string    `json:"type,omitempty"`     // bash, ssh, pwsh, …
    SSHProfileID string    `json:"sshProfileId,omitempty"`
    StartedAt    time.Time `json:"startedAt,omitempty"`
    UpdatedAt    time.Time `json:"updatedAt,omitempty"`
}

type Entry struct {
    ResumeID string    `json:"resumeId"`
    Size     int64     `json:"size"`
    ModTime  time.Time `json:"modTime"`
    Metadata Metadata  `json:"metadata"`
}
```

### 4.2 Funktionen

| Funktion | Verantwortung |
|---|---|
| `Append(resumeID, data) (path, err)` | Hängt Bytes ans `<resumeID>.log` an. Erzeugt die Datei, falls sie nicht existiert. Leerer `data` ist No-op. |
| `ReadTail(resumeID, maxBytes) (text, err)` | Liest die letzten `maxBytes` Bytes der `.log`. Fehlende Datei → leerer String, kein Fehler. |
| `ReadFull(resumeID, maxBytes) (text, err)` | Wie `ReadTail`; semantisch „komplette Datei, gecappt bei `maxBytes`". `maxBytes=0` heißt aus Sicht der Wails-Schicht „bis zur Hard-Obergrenze". |
| `WriteMetadata(resumeID, meta) error` | Schreibt Side-Car. Bewahrt `StartedAt` aus dem vorherigen Stand. **No-op**, wenn Name/Type/SSHProfileID unverändert sind. |
| `ReadMetadata(resumeID) (meta, err)` | Liest Side-Car. Fehlende Datei → Zero-Wert ohne Fehler. Kaputtes JSON → echter Fehler. |
| `List() ([]Entry, err)` | Scannt das Transcript-Verzeichnis, filtert `.log`-Dateien mit validem `resumeID`-Pattern, joined mit `ReadMetadata`. Sortiert absteigend nach `ModTime`. Kaputte Side-Cars werden geloggt, der Entry bleibt mit Zero-Metadata in der Liste. |
| `PathForResumeID(resumeID) (path, err)` | Validierender Pfad-Resolver, intern verwendet. |

### 4.3 Sicherheitsannahmen

- Schreibrechte auf `~/.config/mimir/transcripts/` werden auf `0700`/`0600` gesetzt
- ResumeID wird streng validiert (Regex), keine Path-Traversal möglich
- Schreibvorgänge sind **nicht** atomar (`os.O_APPEND`-Mode auf der `.log`,
  `os.WriteFile` auf der `.json`). Crash mid-write kann eine Teildatei
  hinterlassen — bei der `.log` ist das harmlos (Append-Stream), bei der
  `.json` kann's das JSON beschädigen. Das wird in Phase 5 (siehe unten)
  via `safeio.AtomicWriteFile` adressiert.

---

## 5. Wails-Bindings (`app.go`)

Vier Methoden auf `*App`:

| Methode | Signatur | Aufgerufen von |
|---|---|---|
| `AppendTerminalTranscript(resumeID, data string) error` | per Terminal-Output-Event | `App.svelte` `terminal-output-${id}` Listener |
| `GetTerminalTranscriptExcerpt(resumeID string, maxBytes int) (string, error)` | für die „Restored Transcript"-Overlay-Box | `loadTranscriptExcerpt()` |
| `GetTerminalTranscriptFull(resumeID string, maxBytes int) (string, error)` | Default-Cap 10 MiB | TranscriptViewerModal |
| `ListTranscripts() ([]TranscriptListEntry, error)` | TranscriptViewerModal |
| `SaveTranscriptMetadata(resumeID, name, terminalType, sshProfileID string) error` | beim Anlegen + Rename eines Terminals |

### 5.1 `TranscriptListEntry` und Label-Merge

`ListTranscripts` joined die rohen `transcript.List()`-Einträge mit
zwei zusätzlichen Quellen, in dieser Priorität:

1. **Active in-memory State** (`app.activeTerminalStates`) — frischste Quelle
2. **Saved Session Snapshot** (`session.LoadSession()`) — was beim letzten
   Save bekannt war
3. **Side-Car-Metadata** (`Entry.Metadata`) — überlebt das Schließen eines
   Terminals

Das heißt: ein Terminal, das im laufenden Mimir „API host" heißt, taucht in
der Liste mit „API host" auf. Ist Mimir restartet und das Terminal nicht
mehr offen, kommt das Label aus der Side-Car. Existiert die Side-Car
nicht (z. B. weil das Transcript aus einer Vor-v0.2.4-Version stammt),
zeigt der Viewer „Geschlossenes Terminal".

---

## 6. Frontend-Architektur

Zwei pure Module + eine UI-Komponente:

### 6.1 `lib/transcript/transcriptApi.js`

Dünner Adapter über `window.go.main.App.X`. Einzige Stelle im Code, die
direkt mit der Wails-Bridge spricht. Exports:

```js
listTranscripts()                              → Entry[]
getFullTranscript(resumeId, maxBytes = 0)     → string
getTranscriptExcerpt(resumeId, maxBytes)      → string
appendTerminalTranscript(resumeId, data)      → void (fire-and-forget)
saveTranscriptMetadata({resumeId,name,type,sshProfileId}) → void
```

Alle Funktionen sind safe gegen fehlendes Backend (Dev-Modus, Smoke-Tests).

### 6.2 `lib/transcript/cleanTranscript.js`

Pure Pipeline ohne Svelte-Abhängigkeit. Drei Stufen, in dieser Reihenfolge:

```
stripAnsi          →  Entfernt CSI / OSC / DEC-private / Charset-Selektoren
                      und C0-Steuerbytes außer TAB/LF/CR.
applyCarriageReturns → Innerhalb einer Zeile bleibt nur das, was nach dem
                      letzten \r kommt. Damit werden PSReadLine-Redraws
                      ("p\rpw\rpwd" → "pwd") sinnvoll aufgelöst.
collapseRepeats    →  Faltet runs von ≥ 4 identischen non-blank Zeilen
                      zu „⟨N× more identical⟩". Runs von blanks zu einem
                      Blank, ohne Marker.
```

Zentraler Composer: `cleanTranscript(text)`.

Vollständig unit-getestet (`cleanTranscript.test.js`, 22 Tests).

### 6.3 `lib/modals/TranscriptViewerModal.svelte`

Master-Detail-Modal:

- **Header**: Titel, dynamischer Subtitle (Label des gerade angezeigten
  Eintrags), „Liste (N)"-Toggle, ✕
- **Liste links** (collapsible, default zu wenn man mit einem konkreten
  Terminal-Knopf öffnet): chronologisch sortiert, Label + relativer
  Zeitstempel + Größe
- **Viewer rechts**: `<pre>` mit `displayText`, Auto-Scroll-to-end nach
  Load, Truncation-Banner wenn Hard-Cap erreicht
- **Footer**: „Show raw ANSI"-Checkbox, „Copy All", „Close"

Verhalten:

- **Outside-Click schließt** (Document-level `mousedown`/`touchstart` in
  der Capture-Phase, damit xterm Events nicht vorher abfängt)
- **Escape schließt**
- **Klick auf Listen-Eintrag** schließt die Liste automatisch und zeigt
  den ausgewählten Inhalt voll
- **Stale-Load-Schutz**: jeder Load bekommt einen monoton wachsenden
  Token; ältere Antworten werden verworfen, wenn der User schneller
  klickt als die IPC-Roundtrip dauert

---

## 7. Datenfluss-Szenarien

### 7.1 Output schreiben (Hot-Path)

```
xterm.js data event
       ↓
App.svelte: EventsOn('terminal-output-${id}')
       ↓
appendTerminalTranscript(resumeId, data)  // fire-and-forget
       ↓ (Wails IPC)
App.AppendTerminalTranscript(resumeId, data)
       ↓
transcript.Append → open(O_APPEND) → write → close
```

Aktuell pro Output-Event ein Open/Write/Close. Bei sustained
High-Frequency-Output (Log-Tail) ist das aufgenommen, aber nicht
kritisch — Burst-tolerant. Siehe Follow-up #1.

### 7.2 Terminal anlegen → Side-Car

```
addTerminal() in App.svelte
       ↓
generateResumeId() + Terminal-Zustand
       ↓
saveTranscriptMetadata({resumeId, name, type, sshProfileId})
       ↓ (Wails IPC)
App.SaveTranscriptMetadata(...)
       ↓
transcript.WriteMetadata
       ↓
Compare-mit-existing → wenn unverändert: return early
                    → sonst: marshal + write 0600
```

### 7.3 Viewer öffnen

```
User klickt 📜-Button im Terminal-Header
       ↓
SplitPane dispatch('opentranscript', terminalId)
       ↓
App.svelte openTranscriptViewer(id)
       ↓
transcriptViewerState = {resumeId, label}
       ↓
TranscriptViewerModal mounts
       ↓
onMount: listTranscripts() + (initialResumeId ? loadTranscript(...) : nothing)
       ↓
Reaktiv: $: if (selectedResumeId) loadTranscript(selectedResumeId)
       ↓
loadTranscript: token++ → getFullTranscript(resumeId, 0)
       ↓
text → cleanTranscript() → <pre>
```

---

## 8. Cleaning-Pipeline im Detail

PowerShell, bash, zsh und tmux schreiben jeden Tastendruck als Mischung
aus „neuer Text" + Cursor-Bewegungen in den Stream. Ein Terminal-Emulator
interpretiert das live; ein `<pre>` zeigt die rohen Bytes.

### Was die Pipeline löst

| Symptom im Raw-Stream | Cleaner zeigt |
|---|---|
| `\x1B[32mt3 ❯\x1B[0m pwd` (Farb-Codes) | `t3 ❯ pwd` |
| `\x1B]0;user@host\x07prompt$` (OSC Title) | `prompt$` |
| `\x1B[2J\x1B[H` (Clear Screen) | (entfernt) |
| `p\rpw\rpwd` (PSReadLine-Redraw) | `pwd` |
| 200× `t3 ❯ ` in Folge (Resize-Redraws) | `t3 ❯ ` + `⟨199× more identical⟩` |
| Mehrere Leerzeilen in Folge | eine Leerzeile |

### Was die Pipeline *nicht* löst

PSReadLines „Predictive Intellisense" schreibt grauen Vorschlagstext
inline und löscht ihn dann mit `\x1B[K` (erase-to-end). Das `\x1B[K` wird
gestrippt, der inline-geschriebene Vorschlag bleibt aber sichtbar. Echte
Lösung wäre ein headless VT-Emulator (z. B. `xterm-headless`), der den
Stream live durchspielt und nur den finalen visible buffer dumpt. Siehe
Follow-up #2.

Für solche Fälle ist die „Show raw ANSI"-Checkbox der Notausgang —
zeigt den unveränderten Bytestream.

---

## 9. Tests

| Layer | Suite | Anzahl | Was wird abgedeckt |
|---|---|---|---|
| Go | `transcript/transcript_test.go` | 10 | Append/Read-Roundtrip, ResumeID-Validierung, List-Ordering, Metadata-Roundtrip + StartedAt-Preserve + No-op-on-unchanged |
| JS Unit | `cleanTranscript.test.js` | 22 | Alle vier Cleaning-Funktionen + komponierte Pipeline mit realistischem PowerShell-Snippet |
| JS E2E | `mimir-smoke.spec.js` | 1 von 8 | Modal öffnen, Browse-Toggle, Listen-Wechsel, Close-via-X |

Alle drei Suites laufen in CI auf Ubuntu/Windows/macOS.

---

## 10. Sicherheitserwägungen

- **Roher Output, kein Scrubbing**: Aktuell wird der Stream eins-zu-eins
  geschrieben. Tippt der User ein Passwort interaktiv (z. B. `sudo`), kann
  es als Echo im Transcript landen. Die `recording/scrubber.go`-Pattern
  existieren bereits — das Anhängen im Append-Pfad ist als Follow-up #3
  geplant.
- **0600-Permissions** auf beiden Dateien — nur der User kann lesen.
- **Pfad-Validierung** verhindert Schreiben außerhalb des
  Transcripts-Verzeichnisses.
- **Hard-Cap 10 MiB** beim Read über Wails-IPC, damit pathologische
  Transcripts nicht den Renderer blockieren.

---

## 11. Bekannte Grenzen

- **Keine Rotation / Garbage Collection**: Lang laufende Installationen
  akkumulieren Transcripts unbegrenzt. Manuelles Löschen über das
  Dateisystem ist aktuell der Weg.
- **Append pro Output-Event** ist Open/Close-pro-Call. Bei sustained
  High-Frequency-Streams nicht ideal, aber bisher nicht kritisch.
- **Predictive-Ghost-Residue** (siehe Cleaning-Pipeline-Limitationen).
- **Side-Car-Migration**: Transcripts aus Versionen vor v0.2.4 haben keine
  Side-Car-Datei. Sie zeigen sich im Viewer als „Geschlossenes Terminal"
  mit Größe + Datum, mehr ist nicht wiederherzustellen.

---

## 12. Geplante Follow-ups (priorisiert)

1. **Scrubbing on Append** — `recording.Scrubber` an `Append` hängen,
   damit Passwörter / Tokens redacted werden, bevor sie auf Platte landen.
   Default-on, Toggle in Settings.
2. **VT-Emulator für Clean-View** — `xterm-headless` als Read-Path-Filter
   einbauen für saubere PSReadLine-Output.
3. **Delete-Affordance im Viewer** — Pro-Eintrag-Mülltonne, plus „Alle
   älter als N Tage löschen" als Bulk-Aktion.
4. **Rotation / Retention** — konfigurierbarer Default (z. B. „keep
   last 30 days") plus Disk-Usage-Anzeige in Settings.
5. **Atomic Side-Car-Writes** über `safeio.AtomicWriteFile`.
6. **Append-Buffering** im Backend — pro Resume-ID eine offene FD halten,
   periodisches Flush, Close auf Idle-Timeout.
7. **`SaveTranscriptMetadata` Struct-Param** — fünfter Parameter wird
   irgendwann nötig (z. B. CWD, StartedHost), dann brechen die positional
   Strings. Aufruf umstellen auf `{...}`-Struct ist 5 Zeilen.

---

## 13. Verwandte Dokumente

- [`architecture.md`](architecture.md) — Gesamtbild der App
- [`reboot-workspace-restore.md`](reboot-workspace-restore.md) — der größere
  Plan, in den das Transcript-Modul später integriert wird
- [`security-notes.md`](security-notes.md) — generelle Security-Linie
- [`testing.md`](testing.md) — Vitest/Playwright-Setup
