package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"mimir/safeio"
)

// Images pasted into a pane.
//
// Claude Code and OpenCode take an image from the system clipboard when
// they see Ctrl+V, which is what a local pane forwards. A program on an
// SSH host cannot see this machine's clipboard, so the frontend sends the
// image here, it is stored where the program can read it, and the path is
// pasted into the pane: both agents attach an image whose path appears in
// the prompt. Files are private (0700 directory, 0600 file) and swept
// after a week.

const (
	pastedImageMaxBytes = 32 << 20
	pastedImageKeep     = 7 * 24 * time.Hour
	pastedImagePrefix   = "paste-"
)

var pastedImageExt = map[string]string{
	"image/png":  "png",
	"image/jpeg": "jpg",
	"image/gif":  "gif",
	"image/webp": "webp",
	"image/bmp":  "bmp",
}

// pastedImageName is paste-<UTC time>-<hex>.<ext>. An unknown MIME type is
// refused rather than guessed, so nothing but an image lands in the cache.
func pastedImageName(mime string, now time.Time, random []byte) (string, error) {
	ext, ok := pastedImageExt[strings.ToLower(strings.TrimSpace(mime))]
	if !ok {
		return "", fmt.Errorf("unsupported image type %q", mime)
	}
	return fmt.Sprintf("%s%s-%s.%s", pastedImagePrefix, now.UTC().Format("20060102-150405"), hex.EncodeToString(random), ext), nil
}

func decodePastedImage(dataBase64 string) ([]byte, error) {
	if len(dataBase64) > pastedImageMaxBytes*4/3+4 {
		return nil, fmt.Errorf("image larger than %d MB", pastedImageMaxBytes>>20)
	}
	data, err := base64.StdEncoding.DecodeString(dataBase64)
	if err != nil {
		return nil, fmt.Errorf("image data is not base64: %w", err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("image is empty")
	}
	return data, nil
}

func localPasteDir() (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cache, "mimir", "paste"), nil
}

func savePastedImageLocal(dir, name string, data []byte, now time.Time) (string, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	p := filepath.Join(dir, name)
	if err := safeio.AtomicWriteFile(p, data, 0600); err != nil {
		return "", err
	}
	entries, err := os.ReadDir(dir)
	if err == nil {
		for _, e := range entries {
			info, infoErr := e.Info()
			if infoErr != nil || !isStalePaste(info, now) {
				continue
			}
			if rmErr := os.Remove(filepath.Join(dir, e.Name())); rmErr != nil {
				log.Printf("paste: sweep %s: %v", e.Name(), rmErr)
			}
		}
	}
	return p, nil
}

// isStalePaste matches files this code wrote whose keep time has passed.
func isStalePaste(info os.FileInfo, now time.Time) bool {
	return info != nil && !info.IsDir() && strings.HasPrefix(info.Name(), pastedImagePrefix) && now.Sub(info.ModTime()) > pastedImageKeep
}

func (a *App) savePastedImageRemote(terminalID int, name string, data []byte, now time.Time) (string, error) {
	sc, err := a.remoteFileClient(terminalID)
	if err != nil {
		return "", err
	}
	defer sc.Close()

	home, err := sc.Getwd()
	if err != nil {
		return "", fmt.Errorf("remote home dir: %w", err)
	}
	dir := path.Join(home, ".cache", "mimir", "paste")
	if err := sc.MkdirAll(dir); err != nil {
		return "", fmt.Errorf("create %s: %w", dir, err)
	}
	if err := sc.Chmod(dir, 0700); err != nil {
		log.Printf("paste: chmod %s: %v", dir, err)
	}
	p := path.Join(dir, name)
	f, err := sc.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return "", fmt.Errorf("create %s: %w", p, err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return "", fmt.Errorf("write %s: %w", p, err)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("write %s: %w", p, err)
	}
	if err := sc.Chmod(p, 0600); err != nil {
		log.Printf("paste: chmod %s: %v", p, err)
	}
	if entries, err := sc.ReadDir(dir); err == nil {
		for _, info := range entries {
			if !isStalePaste(info, now) {
				continue
			}
			if rmErr := sc.Remove(path.Join(dir, info.Name())); rmErr != nil {
				log.Printf("paste: sweep %s: %v", info.Name(), rmErr)
			}
		}
	}
	return p, nil
}

func (a *App) isSSHTerminal(terminalID int) bool {
	if a.sftpClientForTerminal != nil {
		return true
	}
	return a.TerminalManager != nil && a.TerminalManager.GetSSHClient(terminalID) != nil
}

// SavePastedImage stores an image from the clipboard where the pane's
// program can read it and returns the path to paste: on the SSH host for
// an SSH pane (via its SFTP connection), in the local cache otherwise.
// mime picks the extension; dataBase64 is the image.
func (a *App) SavePastedImage(terminalID int, mime string, dataBase64 string) (string, error) {
	data, err := decodePastedImage(dataBase64)
	if err != nil {
		return "", err
	}
	random := make([]byte, 4)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	now := time.Now()
	name, err := pastedImageName(mime, now, random)
	if err != nil {
		return "", err
	}
	if a.isSSHTerminal(terminalID) {
		return a.savePastedImageRemote(terminalID, name, data, now)
	}
	dir, err := localPasteDir()
	if err != nil {
		return "", err
	}
	return savePastedImageLocal(dir, name, data, now)
}
