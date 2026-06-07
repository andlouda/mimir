# frontend/src/lib

## Zweck
Wiederverwendbare Svelte-Komponenten, die von `App.svelte` eingebunden werden.

## Inhalt
- `FileBrowser.svelte` - Dateisystem-Browser mit Navigation, Dateivorschau und "Open Terminal Here"-Funktion
- `TemplateManager.svelte` - CRUD-Oberfläche für Befehlsvorlagen (Create, Edit, Delete, Toggle Favorite)

## Verantwortlichkeiten

### FileBrowser.svelte
- Verzeichnisnavigation (auf/ab)
- Dateianzeige (Name, Typ, Sortierung: Ordner zuerst)
- Dateiinhalt-Vorschau (Modal)
- "Open in Explorer"-Funktion (OS-nativer Dateimanager)
- "Open Terminal Here"-Event an Parent dispatchen
- Pfad-Normalisierung (Windows/Linux Kompatibilität)

### TemplateManager.svelte
- Formular zum Erstellen/Bearbeiten von Templates
- Synchronisation von Bash/Zsh/WSL-Befehlen
- Template-Liste mit Favoriten-Toggle
- Löschen von Templates mit Bestätigungsdialog
- Event-Dispatching an Parent (`templateUpdated`, `editTemplate`, `backToTerminals`)

## Abhängigkeiten
- Wails Go-Bindings: `../../wailsjs/go/main/App` (Backend-Funktionsaufrufe)
- Svelte Event-System (`createEventDispatcher`)
- Keine npm-Dependencies

## Wichtige Hinweise
- **TemplateManager Props:** Erhält Backend-Funktionen (`SaveTemplate`, `UpdateTemplate`, etc.) als Props von `App.svelte`
- **FileBrowser Initial Path:** Lädt beim Mount das aktuelle Arbeitsverzeichnis via `GetCurrentDirectory()`
- **Event-Flow:** Beide Komponenten kommunizieren via Svelte Events mit `App.svelte`, nicht direkt miteinander
- **Kein State Store:** Es gibt keinen globalen Svelte Store - der State liegt in `App.svelte`
