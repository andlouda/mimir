# Transcript Stabilization Plan

Status: Phase 2 — Plan (Findings priorisiert, noch keine produktiven Änderungen)
Letzte Aktualisierung: 2026-06-06
Bezogen auf: `transcript/`, `app.go` (Transcript-Bindings), `frontend/src/lib/transcript/`, `frontend/src/lib/modals/TranscriptViewerModal.svelte`

Dieses Dokument ist Voraussetzung für Phase 3. Jeder Punkt wird vor der
Implementierung gegen den tatsächlichen Code validiert und mit Tests
abgesichert. Änderungen erfolgen inkrementell und mit klar lokalisiertem
Blast-Radius.

Legende für `Status`:
- `planned` — noch nicht angefasst
- `in-progress`
- `done`
- `deferred` — bewusst nicht in diesem Pass, mit Begründung

---

## Kritisch (P0)

### P0-1 — Side-Car-Metadata-Write ist nicht atomar

**Problem**
`transcript.WriteMetadata` schreibt mit `os.WriteFile` direkt in `<resumeID>.json`.
Ein Crash mitten im Write kann eine teildateigeschriebene oder leere JSON
hinterlassen, die beim nächsten `ReadMetadata`-Aufruf als „failed to parse"
erkannt wird — der Eintrag verliert sein Label dauerhaft.

**Evidenz**
`transcript/transcript.go` Z. 99: `os.WriteFile(path, data, 0o600)`. Vergleich:
`safeio.AtomicWriteFile` existiert bereits und macht write-temp + rename in
einem atomaren Schritt, mit `Sync()` vor dem Rename.

**Auswirkung**
Datenverlust (Labels), keine harten Datenkorruption (`.log` bleibt OK).

**Lösung**
- `WriteMetadata` auf `safeio.AtomicWriteFile` umstellen
- Permissions auf `0o600` belassen
- Existierende „no-op bei Identity-Save"-Optimierung beibehalten

**Files**
- `transcript/transcript.go`

**Tests**
- Vorhandener `TestWriteMetadataIsNoopWhenContentUnchanged` muss weiterhin
  grün sein
- Neuer Test: nach Pseudo-Crash (Lese-Probe des `.tmp-*`-Patterns) keine
  beschädigte Ziel-Datei
- Atomic-Write-Test: WriteMetadata → ReadMetadata-Roundtrip OK

**Risiko**
Niedrig. `safeio.AtomicWriteFile` ist in der App etabliert und gut getestet.

**Status:** `planned`

---

### P0-2 — Truncation-Detection auf Frontend-Seite ist UTF-8-unsicher

**Problem**
Im Modal:
```js
truncated = Boolean(entry && entry.size > transcriptText.length);
```
`entry.size` ist Datei-Bytes, `transcriptText.length` ist JavaScript-Char-Länge
(UTF-16 Code Units). Bei multi-byte UTF-8 (Umlaute, Emojis, asiatische
Schriften) ist `text.length < size` immer, → **falsches Truncation-Banner**
bei jedem Transcript mit Non-ASCII-Content.

**Evidenz**
`frontend/src/lib/modals/TranscriptViewerModal.svelte` Z. 105.
`mimir.go-mimir-2` Backend cappt bei 10 MiB — keine Wahrheit über tatsächlich
truncierten Inhalt im IPC-Ergebnis.

**Auswirkung**
False positive in der UI; Nutzer denkt, der Transcript sei abgeschnitten,
obwohl er vollständig ist.

**Lösung**
Strukturierte Backend-Antwort statt nackter String:
```go
type TranscriptContent struct {
    ResumeID  string `json:"resumeId"`
    Text      string `json:"text"`
    Size      int64  `json:"size"`       // Datei-Größe in Bytes
    ReadBytes int64  `json:"readBytes"`  // tatsächlich gelesene Bytes
    Truncated bool   `json:"truncated"`  // Backend-authoritative
}
```
Backend setzt `Truncated`, Frontend zeigt nur dann das Banner.

**Files**
- `transcript/transcript.go` (neue Read-Variante)
- `app.go` (`GetTerminalTranscriptFull` Signaturwechsel)
- `frontend/src/lib/transcript/transcriptApi.js`
- `frontend/src/lib/modals/TranscriptViewerModal.svelte`

**Tests**
- Go: Read-Roundtrip mit Truncation-Marker
- Vitest: Modal verlangt strukturiertes Result, Banner nur wenn `truncated===true`
- Playwright: Mock returns `{text, truncated:true}` → Banner sichtbar

**Risiko**
Mittel: API-Breaking-Change zwischen Wails und Frontend. Wir koordinieren
beide Seiten in einem Commit; `GetTerminalTranscriptExcerpt` bleibt
unverändert (Backwards-compat für die Restore-Overlay-Box).

**Status:** `planned`

---

### P0-3 — Hard-Cap im Append-Pfad fehlt

**Problem**
`transcript.Append` hat keine Obergrenze für Einzeldateien. Ein Log-Stream
mit `tail -f` über Wochen kann mehrere GB akkumulieren. Beim ersten Read
schickt das Backend brav 10 MiB rüber, aber die Platte ist voll.

**Evidenz**
`transcript/transcript.go` `Append` — kein Größencheck. Keine Retention,
kein Rotate.

**Auswirkung**
Unbegrenztes Disk-Wachstum, kann den User-Config-Mount füllen.

**Lösung — gestuft**
1. Im Append-Pfad: Größencheck vor Write. Wenn Datei > Maximum (z. B. 100 MiB
   konfigurierbar), neue Writes verwerfen mit `transcript: file size limit
   reached` Fehler — der Frontend-Fire-and-Forget-Pfad ignoriert das, aber wir
   loggen einmal pro Minute pro resumeID (rate-limited).
2. Retention-API (siehe P0-4) — periodische Bereinigung.

**Files**
- `transcript/transcript.go`
- `app.go` (optional: Settings-Hook für Limit)

**Tests**
- Go: Append über Limit hinaus → Datei wächst nicht weiter, kein Fehler an
  Caller, Rate-limited Log
- Go: Limit-Konfiguration über Setter, default 100 MiB

**Risiko**
Niedrig. Verändert kein bestehendes Read-Verhalten.

**Status:** `planned`

---

### P0-4 — Keine Retention, keine Disk-Usage-Anzeige, kein Delete

**Problem**
Der User hat aktuell 80+ Transcripts in seinem Verzeichnis (verifiziert), kein
UI-Weg um die alten zu löschen. Unbegrenztes Wachstum + keine Diagnostik.

**Evidenz**
Kein Delete in `transcript/`, kein Disk-Usage-Endpoint, kein Retention-Sweep.

**Auswirkung**
Datenschutz (sehr alte Sessions mit potentiell sensiblen Inhalten bleiben
erhalten), Disk-Usage, schlechte UX („Liste mit 80 Einträgen die alle
'Geschlossenes Terminal' heißen").

**Lösung**
Backend-API:
```go
func Delete(resumeID string) error                       // einzeln
func DeleteMany(resumeIDs []string) ([]DeleteResult, error)
func DeleteOlderThan(maxAge time.Duration) (int, error)  // bulk by age
func DiskUsage() (DiskUsageInfo, error)                  // {count, totalBytes}
```
- Aktive Transcripts (resumeIDs in `activeTerminalStates`) sind **geschützt**
  und können nicht gelöscht werden — Schutz im Backend, nicht nur in der UI.
- Pfad-Validierung über die existierende `resumeIDPattern` + Pfad-Containment-
  Check (`filepath.Rel(dir, target)` darf keine `..` enthalten).
- Side-Car wird mitgelöscht.

Wails-Bindings:
- `DeleteTranscript(resumeID)`
- `DeleteTranscripts(resumeIDs)`
- `GetTranscriptDiskUsage()`

UI (Phase 4):
- Einzel-Delete-Button pro Listen-Eintrag
- Bulk-Delete im Browse-Modus mit Checkbox
- Disk-Usage in der Modal-Footer-Zeile
- Explizite Bestätigung; kein Auto-Delete

**Files**
- `transcript/transcript.go`
- `app.go`
- `frontend/src/lib/transcript/transcriptApi.js`
- `frontend/src/lib/modals/TranscriptViewerModal.svelte` (Phase 4)

**Tests**
- Go: Single-Delete, Many-Delete, Older-than, aktive Transcripts geschützt,
  Pfad-Traversal-Versuch abgelehnt
- Vitest: Delete-Confirmation-Flow, Disable-State wenn aktiv
- Playwright: Delete-Button, Bestätigungsdialog

**Risiko**
Mittel. Delete-Operationen sind nicht rückgängig zu machen — die Tests müssen
die Schutzpfade abdecken. Pfad-Validierung muss wasserdicht sein.

**Status:** `planned`

---

## Hoch (P1)

### P1-1 — `ReadTail` liest die ganze Datei statt nur den Tail

**Problem**
```go
data, err := os.ReadFile(path)   // ganze Datei in Memory
if maxBytes > 0 && len(data) > maxBytes {
    data = data[len(data)-maxBytes:]
}
```
Für die `restored-transcript`-Overlay (8 KB Tail aus möglicherweise 50 MB Datei)
laden wir 50 MB in den Speicher.

**Evidenz**
`transcript/transcript.go` Z. 146-162.

**Auswirkung**
Speicher-Spike beim Restore-Pfad, langsamer App-Start nach langen Sessions.

**Lösung**
Bei `maxBytes > 0 && < dateigröße`: `os.OpenFile` + `Seek(-maxBytes, io.SeekEnd)`
+ `io.ReadAll`. Plus UTF-8-Grenz-Korrektur: ersten Bytes nach Seek auf gültigen
UTF-8-Anfang justieren (skip continuation bytes).

**Files**
- `transcript/transcript.go`

**Tests**
- Go: Tail-Read liest nur maxBytes Bytes (Mock-IO oder Größencheck nach Read)
- Go: Tail-Read schneidet keine UTF-8-Mehrbyte-Sequenz mittig durch
- Go: Tail-Read kleiner Dateien (< maxBytes) liest komplett

**Risiko**
Niedrig. Lokaler Read-Pfad-Refactor, alle existierenden Tests bleiben gültig.

**Status:** `planned`

---

### P1-2 — Append ist nicht garantiert atomisch unter paralleler Last

**Problem**
`os.OpenFile(O_APPEND|O_CREATE|O_WRONLY)` + `WriteString` ist auf POSIX
atomar **nur bis PIPE_BUF** (4 KB Linux). Auf Windows existiert das Konzept
nicht — parallele Appends können sich interleaven.

Aktuell ist der einzige Caller der Wails-Bridge-Aufruf aus dem Frontend, und
Wails serialisiert standardmäßig pro Methode … oder?

**Evidenz**
`transcript/transcript.go` `Append`. Frontend ruft pro Output-Event auf
(`App.svelte:961` via `appendTerminalTranscript`). Bei 10 parallelen
Terminals mit hohem Output kommen Calls quasi-gleichzeitig an.

**Auswirkung**
Theoretisch: korrupte Zeilen bei Last. In der Praxis selten — die Writes sind
typischerweise wenige hundert Bytes (PTY-Chunks).

**Lösung**
Pro `resumeID` ein `sync.Mutex` im Append-Pfad. Cheap, eliminiert die Frage
vollständig.

**Files**
- `transcript/transcript.go`

**Tests**
- Go: Parallel-Append-Test (100 Goroutinen, je 1000 Writes pro Goroutine,
  prüfen ob jeder Write atomisch ist)

**Risiko**
Niedrig. Pro-Key-Mutex ist Standard.

**Status:** `planned`

---

### P1-3 — Doppel-Load beim Modal-Öffnen

**Problem**
Im Modal:
```js
onMount(async () => {
  ...
  await loadList();                     // → entries set + selectedResumeId set
  if (selectedResumeId) await loadTranscript(selectedResumeId);  // (a)
});
$: if (selectedResumeId) loadTranscript(selectedResumeId);       // (b)
```
Wenn das Modal ohne `initialResumeId` aufgeht, dann setzt `loadList()` den
`selectedResumeId` reaktiv → (b) feuert → gleichzeitig läuft (a). Zwei
parallele IPC-Calls, die das Token-Pattern abfängt — aber Backend macht
doppelte Arbeit.

**Evidenz**
`TranscriptViewerModal.svelte` Z. 150-162.

**Auswirkung**
Verschwendete IPC-Roundtrips + Disk-Reads beim Initial-Open.

**Lösung**
Explizites Load entfernen aus `onMount` — die reaktive Aussage allein lädt.
ODER reaktive Aussage entfernen, nur explizit laden bei `select()`.
Variante A ist sauberer (Reaktivität ist die kanonische Quelle).

**Files**
- `frontend/src/lib/modals/TranscriptViewerModal.svelte`

**Tests**
- Vitest: Modal-Mount-Test (mit DOM-Fake) — `loadTranscript` exactly 1 mal
  gerufen für Initial-Open ohne `initialResumeId`

**Risiko**
Niedrig. Lokale Modal-Logik.

**Status:** `planned`

---

### P1-4 — Fokus-Management des Modals fehlt vollständig

**Problem**
- Kein Auto-Focus auf Modal beim Öffnen → Screen-Reader bekommen keinen
  Hinweis
- Kein Focus-Trap → Tab bringt Fokus auf darunterliegende Terminals/Sidebar
- Kein Focus-Restore beim Schließen → Fokus geht verloren

**Evidenz**
`TranscriptViewerModal.svelte` — `tabindex="-1"` am Dialog, aber kein
`element.focus()` in `onMount`. Kein Trap-Handler.

**Auswirkung**
A11y-Regression, Keyboard-User können das Modal kaum nutzen.

**Lösung**
- `onMount`: `triggerEl = document.activeElement; await tick(); modalEl?.focus();`
- Focus-Trap: Tab/Shift+Tab in der Capture-Phase, Loop am ersten/letzten
  fokussierbaren Element
- `onDestroy`: `triggerEl?.focus()` (mit Existenz-Check)

**Files**
- `frontend/src/lib/modals/TranscriptViewerModal.svelte`

**Tests**
- Playwright: Tab cycle bleibt im Modal; Escape gibt Focus an Trigger zurück

**Risiko**
Niedrig.

**Status:** `planned`

---

### P1-5 — Keyboard-Navigation in der Liste fehlt

**Problem**
ArrowUp/Down navigiert nicht. Enter aktiviert nicht. Home/End spring nicht.

**Lösung**
ListBox-Pattern: `role="listbox"` am `<ul>`, `role="option"` pro Eintrag,
`aria-activedescendant` für visuelle Aktiv-Marke, Keydown-Handler an der
Liste.

**Files**
- `frontend/src/lib/modals/TranscriptViewerModal.svelte`

**Tests**
- Vitest oder Playwright: ArrowDown → next, Enter → select, Home → first

**Risiko**
Niedrig.

**Status:** `planned`

---

### P1-6 — Keine Suche im Transcript

**Problem**
Brief verlangt: „Suche innerhalb des Transcripts, Trefferanzahl, Navigation
zum nächsten/vorherigen Treffer."

**Lösung**
Such-Input in der Viewer-Toolbar. Auf Toggle Ctrl+F. Highlight via
`<mark>`-Spans im `<pre>` (nicht `innerHTML` — vorbereitet als Array von
{text, match}-Segments, gerendert via Svelte `{#each}`).

Für 10 MB Text ist Highlight-Markup-Generierung teuer. Strategie:
- Erst Match-Counter über `indexOf` Loop (cheap)
- Render-Highlight nur für sichtbare Page (Scroll-Listener)
- Oder: gar kein Inline-Highlight, nur Scroll-zu-Treffer + Cursor-Markierung
  (browser-natives Find-on-Page Effekt mit Selection-API)

Wegen Performance-Risiko: V1 = Browser-Selection-API (Selection-Range setzen,
scrollIntoView). Inline-Highlight als V2 mit Virtualisierung.

**Files**
- `frontend/src/lib/modals/TranscriptViewerModal.svelte`

**Tests**
- Vitest: Match-Counter korrekt
- Playwright: Cmd+F öffnet Suche, Enter springt zum nächsten

**Risiko**
Mittel — naive Implementierung kann bei 10 MB freezen. V1 mit Selection-API
ist sicher.

**Status:** `planned`

---

### P1-7 — Cleaning-Pipeline blockiert Main Thread bei großen Inputs

**Problem**
Vier Regex-`replace`-Passes über bis zu 10 MB synchron auf dem Main Thread.
Auf langsamen Rechnern messbar 100-300 ms Freeze beim Switchen.

**Lösung**
1. Memoize ergebnis (`text` + `mode` → `displayText`): cache last input/output.
2. Bei sehr großen Texten (`> 1 MB`): Loading-State zeigen, dann in
   `requestIdleCallback` rechnen.
3. Web Worker erst, wenn nötig — kostet Build/Bundle-Overhead.

V1: Memoization + Loading-State. Worker als Folge-Iteration.

**Files**
- `frontend/src/lib/modals/TranscriptViewerModal.svelte`

**Tests**
- Vitest: cleanTranscript wird bei identischer Input nicht zweimal gerufen

**Risiko**
Niedrig.

**Status:** `planned`

---

### P1-8 — Retry-Button bei Lade-Fehlern fehlt

**Problem**
Bei `loadTranscript`-Fehler kommt nur eine Error-Bar; kein Weg, nochmal zu
versuchen, ohne das ganze Modal zu schließen.

**Lösung**
Im Pane-Empty-State bei Error: „Erneut versuchen" Button → `loadTranscript()`
neu aufrufen.

**Files**
- `frontend/src/lib/modals/TranscriptViewerModal.svelte`

**Tests**
- Playwright: Mock-Backend wirft → Retry-Button sichtbar → Klick → erneuter
  Call

**Risiko**
Niedrig.

**Status:** `planned`

---

### P1-9 — Scrubbing-Optionalität ist nicht klar

**Problem**
Brief verlangt: „Scrubbing nur, wenn seine Semantik für Terminal-Streams
geeignet ist." Der `recording.Scrubber` ist stateless-Regex-Pass über
String. Über Chunk-Grenzen kann ein Secret zerteilt werden („sk-" in Chunk A,
„abc..." in Chunk B), dann sieht keine Regex es.

**Auswirkung**
Falscher Schutz wäre schlimmer als kein Schutz.

**Entscheidung**
- Scrubbing in v0.2.x **nicht in den Append-Pfad einbauen.**
- Stattdessen: ADR-Eintrag, der die Trade-offs beschreibt und einen
  stateful-Buffered-Ansatz für eine spätere Version skizziert (Sliding-
  Window mit Overlap, gleich der Recording-Strategy).
- Im Viewer: Hinweis-Banner „Transcripts werden unredigiert gespeichert"
  in den Settings-Erklärung (nicht in jedem Open).

**Files**
- `docs/adr/ADR-0014-transcript-scrubbing.md` (neu)
- `docs/security-notes.md` (Update)

**Tests**
- N/A in diesem Pass

**Risiko**
Keiner — wir machen explizit nichts Halbes.

**Status:** `deferred` (in diesem Pass als ADR dokumentiert)

---

## Mittel (P2)

### P2-1 — Strukturierte Fehler statt rohe Go-Strings im Frontend

**Problem**
`onError(\`Failed to load transcripts: ${error?.message}\`)` zeigt rohe Go-
Strings („failed to read transcript directory: open ...: no such file or
directory") direkt im UI.

**Lösung**
Wails-Bindings geben `{code, message, retryable}` zurück. Frontend mapped
`code` auf benutzerfreundliche Strings via i18n.

**Status:** `planned` (Phase 4)

---

### P2-2 — Aktive vs. geschlossene Terminals visuell unterscheiden

**Problem**
Im Backend wissen wir, welche resumeIDs in `activeTerminalStates` stecken,
geben das aber nicht zurück.

**Lösung**
`TranscriptListEntry.Active bool`. UI zeigt Badge „aktiv" und blockiert die
Delete-Aktion.

**Status:** `planned`

---

### P2-3 — Side-Car `UpdatedAt` nicht beim Append aktualisiert

**Problem**
`UpdatedAt` wird nur bei `WriteMetadata` gesetzt. Damit zeigt es das
„letzter Rename"-Datum, nicht das „letzter Output"-Datum. Verwirrend.

**Lösung — Doku-Klärung:** Doku eindeutig: `UpdatedAt = letzter
Metadata-Edit`. `ModTime` der `.log` = letzter Output. UI nutzt `ModTime`.

**Files**
- `transcript/transcript.go` Doc-Comment
- `docs/transcript-module.md`

**Status:** `planned`

---

### P2-4 — Outside-Click-Handler interagiert mit darunterliegenden Modals

**Problem**
Wenn das Transcript-Modal auf einem anderen Modal sitzt, schließen Klicks
auf das untere Modal beide.

**Lösung**
Z-Index-Stack + Top-Modal-Check. Aktuell ist die App so gebaut, dass es
selten überlappende Modals gibt — als Risk-Note dokumentieren, nicht im
Code adressieren.

**Status:** `deferred`

---

### P2-5 — `truncated`-Flag aktualisiert nicht bei Raw/Clean-Switch

**Problem**
Banner zeigt Truncation, auch wenn `displayText` (clean) deutlich kürzer
ist als raw.

**Lösung**
Truncation ist eine Property des **gelesenen** Streams, nicht des Cleaners.
Banner-Text klarer: „Datei ist X MB groß, gelesene Y MB."

**Status:** `planned`

---

### P2-6 — Subtitle inkonsistent während async-Load

**Problem**
Erste Render zeigt `initialLabel` (vom Trigger-Terminal), nach `loadList`
flippt's zum `entryLabel(selectedEntry)`. Kann flackern.

**Lösung**
`subtitle = selectedEntry ? entryLabel(selectedEntry) : initialLabel` ist
schon richtig — Flacker ist kosmetisch. Akzeptabel.

**Status:** `deferred`

---

## Niedrig (P3)

### P3-1 — `formatRelative` ist Snapshot bei Mount

Modal länger als 1 Stunde offen → „vor 30 min" stimmt nicht mehr. Reactivity
nur über `Date.now()`. Mit `setInterval(1min)` lösbar, aber niedrige
Priorität.

**Status:** `deferred`

---

### P3-2 — Browse-Toggle Label mit `(N)` ist Test-fragil

Locale-String enthält `({n})` — Playwright matched via Regex. Bei Übersetzung
in DE kann sich Klammer-Stil ändern.

**Lösung**
Konsistente i18n-Konvention. Niedrig.

**Status:** `deferred`

---

### P3-3 — Reduced-Motion und Kontrast-Audit

Kein Audit gegen WCAG-Kontrast-Werte. Modal-Farben sind dunkel, sollten
aber AA passen. Mit `prefers-reduced-motion` keine Anpassung; aktuell gibt
es kaum Animationen im Modal.

**Status:** `deferred`

---

## Vorgeschlagene Implementierungs-Reihenfolge

Phase 3 in dieser Sequenz, jeweils mit Tests vor dem nächsten Schritt:

1. **P0-1** Atomic Side-Car (lokal, niedriges Risiko, Test-first)
2. **P1-2** Per-resumeID Mutex im Append (lokal, Test-first)
3. **P0-2** Strukturierte `TranscriptContent` API (Backend + Wails + Frontend
   in EINEM Commit) — und P2-5 (Truncation-Banner-Text-Update) gleich mit
4. **P1-1** Tail-Seek-Read + UTF-8-Grenze
5. **P0-3** Append-Size-Limit (Standard 100 MiB)
6. **P0-4** Delete-API (single, many, older-than) + Disk-Usage + Active-Schutz
7. **P2-2** `Active`-Flag in `TranscriptListEntry`

Phase 4 (UI/UX) danach:

8. **P1-3** Doppel-Load-Fix beim Modal-Mount
9. **P1-4** Fokus-Management + Focus-Trap
10. **P1-5** Listen-Keyboard-Navigation
11. **P1-7** Memoization + Loading-State für große Inputs
12. **P1-8** Retry-Button
13. **P0-4 UI** Delete-Button + Confirmation + Disk-Usage-Anzeige
14. **P1-6** Suche im Transcript (V1: Selection-API)
15. **P2-1** Strukturierte Fehler + i18n
16. **P2-3 / P2-5** Doku-Klärungen + Banner-Text

Phase 5: ADRs + finaler Report.

---

## Nicht in diesem Pass (Begründungen)

- **Headless VT-Emulator** (xterm-headless) — separater ADR mit
  Performance-Messung. Bundle-Größe und Dependency-Surface zu groß für
  unmessbaren UX-Gewinn ohne A/B-Test.
- **Scrubbing on Append** — siehe P1-9. Chunk-Boundary-Problem; eigener
  ADR + spätere Iteration.
- **Web Worker für Cleaning** — nur wenn Memoization + Idle-Callback nicht
  reichen.
- **Soft-delete (`.trash/`)** — verkompliziert die UX („wo sind meine
  Transcripts?"). Hard-Delete mit Confirmation ist ausreichend.

---

## Risiko-Übersicht für Phase 3

| Item | Test-Coverage erforderlich | Backwards-Compat-Risiko |
|---|---|---|
| P0-1 Atomic Write | Roundtrip + Fault-Injection | keiner |
| P0-2 Structured Content | Backend+Bridge+Frontend Tests | API-Break, in 1 Commit |
| P0-3 Append-Limit | Größentest | keiner |
| P0-4 Delete/Retention | Pfad-Safety + Aktiv-Schutz | neuer Endpoint, keiner |
| P1-1 Tail-Seek + UTF-8 | UTF-8-Grenztest | keiner |
| P1-2 Mutex | Parallel-Test | keiner |
