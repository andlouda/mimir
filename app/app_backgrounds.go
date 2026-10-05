package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"mimir/safeio"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// Per-pane background images.
//
// The user picks a file through the native dialog; Mimir copies it (never
// links it) into <config>/mimir/backgrounds/<sha256-prefix>.<ext>, named
// after its content so the same picture is stored once. The webview loads
// it from the asset server at /mimir-bg/<id>, which only serves ids of
// that exact shape from that one directory. Which pane shows which image,
// and how (opacity, blur, fit), lives in the session next to the pane.

const (
	backgroundsDirName  = "backgrounds"
	backgroundIndexName = "index.json"
	backgroundURLPrefix = "/mimir-bg/"
	backgroundMaxBytes  = 25 << 20
	backgroundJSONMax   = 1024
)

var backgroundIDPattern = regexp.MustCompile(`^[0-9a-f]{16}\.(png|jpg|webp|gif)$`)

// backgroundExtByType maps the sniffed content type to the stored
// extension; the file name the user picked is not trusted.
var backgroundExtByType = map[string]string{
	"image/png":  "png",
	"image/jpeg": "jpg",
	"image/webp": "webp",
	"image/gif":  "gif",
}

// BackgroundImage is one imported picture as listed to the frontend.
type BackgroundImage struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	AddedAt string `json:"addedAt"`
}

func backgroundsDir() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("backgrounds config dir: %w", err)
	}
	dir := filepath.Join(configDir, "mimir", backgroundsDirName)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("create backgrounds dir: %w", err)
	}
	return dir, nil
}

func readBackgroundIndex(dir string) map[string]BackgroundImage {
	index := map[string]BackgroundImage{}
	data, err := os.ReadFile(filepath.Join(dir, backgroundIndexName))
	if err != nil {
		return index
	}
	_ = json.Unmarshal(data, &index)
	return index
}

func writeBackgroundIndex(dir string, index map[string]BackgroundImage) error {
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	return safeio.AtomicWriteFile(filepath.Join(dir, backgroundIndexName), data, 0600)
}

// importBackgroundFile copies one image into dir and returns its record.
// Size and content type are checked before anything is written.
func importBackgroundFile(dir, srcPath string) (BackgroundImage, error) {
	info, err := os.Stat(srcPath)
	if err != nil {
		return BackgroundImage{}, err
	}
	if !info.Mode().IsRegular() {
		return BackgroundImage{}, errors.New("not a regular file")
	}
	if info.Size() > backgroundMaxBytes {
		return BackgroundImage{}, fmt.Errorf("image larger than %d MB", backgroundMaxBytes>>20)
	}
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return BackgroundImage{}, err
	}
	ext, ok := backgroundExtByType[http.DetectContentType(data)]
	if !ok {
		return BackgroundImage{}, errors.New("unsupported image type (PNG, JPEG, WebP or GIF)")
	}
	sum := sha256.Sum256(data)
	id := hex.EncodeToString(sum[:8]) + "." + ext
	if err := safeio.AtomicWriteFile(filepath.Join(dir, id), data, 0600); err != nil {
		return BackgroundImage{}, err
	}
	rec := BackgroundImage{ID: id, Name: filepath.Base(srcPath), Size: int64(len(data)), AddedAt: time.Now().Format(time.RFC3339)}
	index := readBackgroundIndex(dir)
	if prev, ok := index[id]; ok {
		rec.AddedAt = prev.AddedAt // same picture again: keep the first import
	}
	index[id] = rec
	if err := writeBackgroundIndex(dir, index); err != nil {
		return BackgroundImage{}, err
	}
	return rec, nil
}

// ImportTerminalBackground opens the native file dialog and imports the
// chosen picture. Returns the record as JSON, or "" when the user cancels.
func (a *App) ImportTerminalBackground() (string, error) {
	if a.ctx == nil {
		return "", errors.New("app not started")
	}
	path, err := wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "Background image",
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "Images (*.png, *.jpg, *.jpeg, *.webp, *.gif)", Pattern: "*.png;*.jpg;*.jpeg;*.webp;*.gif"},
		},
	})
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(path) == "" {
		return "", nil
	}
	dir, err := backgroundsDir()
	if err != nil {
		return "", err
	}
	rec, err := importBackgroundFile(dir, path)
	if err != nil {
		return "", err
	}
	log.Printf("background image imported: %s (%d bytes) as %s", rec.Name, rec.Size, rec.ID)
	data, _ := json.Marshal(rec)
	return string(data), nil
}

// ListTerminalBackgroundsJSON lists the imported pictures, newest first.
// Records whose file disappeared are dropped from the index.
func (a *App) ListTerminalBackgroundsJSON() (string, error) {
	dir, err := backgroundsDir()
	if err != nil {
		return "[]", err
	}
	index := readBackgroundIndex(dir)
	list := make([]BackgroundImage, 0, len(index))
	changed := false
	for id, rec := range index {
		if !backgroundIDPattern.MatchString(id) {
			delete(index, id)
			changed = true
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, id)); err != nil {
			delete(index, id)
			changed = true
			continue
		}
		rec.ID = id
		list = append(list, rec)
	}
	if changed {
		_ = writeBackgroundIndex(dir, index)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].AddedAt > list[j].AddedAt })
	data, err := json.Marshal(list)
	if err != nil {
		return "[]", err
	}
	return string(data), nil
}

// DeleteTerminalBackground removes an imported picture. Panes that used it
// fall back to the plain background on the frontend side.
func (a *App) DeleteTerminalBackground(id string) error {
	if !backgroundIDPattern.MatchString(id) {
		return errors.New("invalid background id")
	}
	dir, err := backgroundsDir()
	if err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(dir, id)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	index := readBackgroundIndex(dir)
	delete(index, id)
	return writeBackgroundIndex(dir, index)
}

// UpdateTerminalBackground stores the pane's background settings (JSON:
// id, opacity, blur, fit) with its session state; "" clears them.
func (a *App) UpdateTerminalBackground(terminalID int, background string) {
	if len(background) > backgroundJSONMax {
		return
	}
	a.stateMu.Lock()
	defer a.stateMu.Unlock()
	state, ok := a.activeTerminalStates[terminalID]
	if !ok {
		return
	}
	state.Background = background
	a.activeTerminalStates[terminalID] = state
}

// backgroundHandler serves /mimir-bg/<id> from the backgrounds directory.
// It is the asset server's fallback, so every other unknown path stays a
// 404 as before.
func backgroundHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}
		id, ok := strings.CutPrefix(r.URL.Path, backgroundURLPrefix)
		if !ok || !backgroundIDPattern.MatchString(id) {
			http.NotFound(w, r)
			return
		}
		dir, err := backgroundsDir()
		if err != nil {
			http.NotFound(w, r)
			return
		}
		f, err := os.Open(filepath.Join(dir, id))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil || !info.Mode().IsRegular() {
			http.NotFound(w, r)
			return
		}
		// Content-addressed: the bytes behind an id never change.
		w.Header().Set("Cache-Control", "max-age=31536000, immutable")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		http.ServeContent(w, r, id, info.ModTime(), f)
	})
}
