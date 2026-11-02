package main

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPastedImageName(t *testing.T) {
	now := time.Date(2026, 11, 2, 14, 5, 6, 0, time.UTC)
	name, err := pastedImageName("image/PNG ", now, []byte{0xde, 0xad})
	if err != nil || name != "paste-20261102-140506-dead.png" {
		t.Fatalf("got %q, %v", name, err)
	}
	if _, err := pastedImageName("text/plain", now, []byte{1}); err == nil {
		t.Fatal("a non-image MIME type must be refused")
	}
	if _, err := pastedImageName("", now, []byte{1}); err == nil {
		t.Fatal("an empty MIME type must be refused")
	}
}

func TestDecodePastedImage(t *testing.T) {
	if _, err := decodePastedImage("not base64!"); err == nil {
		t.Fatal("garbage must be refused")
	}
	if _, err := decodePastedImage(""); err == nil {
		t.Fatal("an empty image must be refused")
	}
	if _, err := decodePastedImage(strings.Repeat("A", pastedImageMaxBytes*4/3+8)); err == nil {
		t.Fatal("an oversized image must be refused")
	}
	got, err := decodePastedImage(base64.StdEncoding.EncodeToString([]byte("png-bytes")))
	if err != nil || string(got) != "png-bytes" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestSavePastedImageLocalWritesPrivatelyAndSweeps(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "paste")
	now := time.Now()
	// A stale paste and a foreign file: only the stale paste goes.
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(dir, "paste-20200101-000000-0000.png")
	foreign := filepath.Join(dir, "notes.txt")
	for _, p := range []string{stale, foreign} {
		if err := os.WriteFile(p, []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
		old := now.Add(-pastedImageKeep - time.Hour)
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}

	p, err := savePastedImageLocal(dir, "paste-20261102-140506-dead.png", []byte("png-bytes"), now)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(p)
	if err != nil || string(data) != "png-bytes" {
		t.Fatalf("read back %q, %v", data, err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale paste should be swept, stat err = %v", err)
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Fatalf("foreign file must survive the sweep: %v", err)
	}
}

func TestSavePastedImageRefusesBadInput(t *testing.T) {
	app := &App{}
	if _, err := app.SavePastedImage(1, "image/png", "***"); err == nil {
		t.Fatal("bad base64 must be refused before anything is written")
	}
	if _, err := app.SavePastedImage(1, "application/pdf", base64.StdEncoding.EncodeToString([]byte("x"))); err == nil {
		t.Fatal("a non-image must be refused")
	}
}
