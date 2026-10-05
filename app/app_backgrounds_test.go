package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func pngBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(1, 1, color.RGBA{R: 200, G: 30, B: 40, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestImportBackgroundFileStoresByContent(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(t.TempDir(), "wallpaper.PNG")
	if err := os.WriteFile(src, pngBytes(t), 0600); err != nil {
		t.Fatal(err)
	}
	rec, err := importBackgroundFile(dir, src)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if !backgroundIDPattern.MatchString(rec.ID) || filepath.Ext(rec.ID) != ".png" {
		t.Fatalf("unexpected id %q", rec.ID)
	}
	if rec.Name != "wallpaper.PNG" {
		t.Fatalf("name %q", rec.Name)
	}
	if _, err := os.Stat(filepath.Join(dir, rec.ID)); err != nil {
		t.Fatalf("file not stored: %v", err)
	}
	// Same picture under another name: same id, index keeps one entry.
	src2 := filepath.Join(t.TempDir(), "copy.png")
	_ = os.WriteFile(src2, pngBytes(t), 0600)
	rec2, err := importBackgroundFile(dir, src2)
	if err != nil || rec2.ID != rec.ID {
		t.Fatalf("second import: %v id=%q want %q", err, rec2.ID, rec.ID)
	}
	if n := len(readBackgroundIndex(dir)); n != 1 {
		t.Fatalf("index entries = %d, want 1", n)
	}
}

func TestImportBackgroundFileRejectsNonImages(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(t.TempDir(), "notes.png")
	_ = os.WriteFile(src, []byte("#!/bin/sh\necho hi\n"), 0600)
	if _, err := importBackgroundFile(dir, src); err == nil {
		t.Fatal("expected a content-type error")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("nothing should be written, got %d entries", len(entries))
	}
}

func TestBackgroundHandlerServesOnlyValidIDs(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	t.Setenv("HOME", cfg)
	t.Setenv("APPDATA", cfg)
	dir, err := backgroundsDir()
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(t.TempDir(), "a.png")
	_ = os.WriteFile(src, pngBytes(t), 0600)
	rec, err := importBackgroundFile(dir, src)
	if err != nil {
		t.Fatal(err)
	}
	h := backgroundHandler()
	cases := map[string]int{
		backgroundURLPrefix + rec.ID:                 http.StatusOK,
		backgroundURLPrefix + "../index.json":        http.StatusNotFound,
		backgroundURLPrefix + "index.json":           http.StatusNotFound,
		backgroundURLPrefix + "0123456789abcdef.svg": http.StatusNotFound,
		"/other/" + rec.ID:                           http.StatusNotFound,
	}
	for path, want := range cases {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != want {
			t.Errorf("%s: status %d, want %d", path, rr.Code, want)
		}
		if want == http.StatusOK && rr.Header().Get("Content-Type") != "image/png" {
			t.Errorf("%s: content type %q", path, rr.Header().Get("Content-Type"))
		}
	}
}
