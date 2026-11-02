package main

import (
	"os"
	"path/filepath"
	"testing"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

func TestFitWindowToScreen(t *testing.T) {
	cases := []struct {
		name         string
		w, h         int
		screen       screenSize
		wantW, wantH int
	}{
		{"1080p at 125 % shrinks the default height", 0, 0, screenSize{1536, 864}, 1228, 744},
		{"1440p at 150 % shrinks the default height", 0, 0, screenSize{1707, 960}, 1228, 840},
		{"4K keeps the default", 0, 0, screenSize{3840, 2160}, 1228, 922},
		{"unknown screen keeps the size", 1400, 1000, screenSize{}, 1400, 1000},
		{"saved size that fits is kept", 1000, 700, screenSize{1536, 864}, 1000, 700},
		{"saved size wider than the screen is shrunk", 2000, 700, screenSize{1536, 864}, 1504, 700},
		{"unusable saved size falls back to the default", 100, 50, screenSize{1536, 864}, 1228, 744},
		{"tiny screen stops at the minimum", 0, 0, screenSize{640, 480}, 640, 400},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w, h := fitWindowToScreen(tc.w, tc.h, tc.screen)
			if w != tc.wantW || h != tc.wantH {
				t.Fatalf("got %dx%d, want %dx%d", w, h, tc.wantW, tc.wantH)
			}
		})
	}
}

func TestPickScreen(t *testing.T) {
	// ScreenSize lives in Wails' internal package; build screens by field.
	screen := func(current, primary bool, w, h int) wailsruntime.Screen {
		s := wailsruntime.Screen{IsCurrent: current, IsPrimary: primary}
		s.Size.Width, s.Size.Height = w, h
		return s
	}
	cases := []struct {
		name    string
		screens []wailsruntime.Screen
		want    screenSize
	}{
		{"none", nil, screenSize{}},
		{"current wins over primary", []wailsruntime.Screen{
			screen(false, true, 1920, 1080),
			screen(true, false, 2560, 1440),
		}, screenSize{2560, 1440}},
		{"primary wins over the first", []wailsruntime.Screen{
			screen(false, false, 1280, 1024),
			screen(false, true, 1920, 1080),
		}, screenSize{1920, 1080}},
		{"first known size without flags", []wailsruntime.Screen{
			screen(false, false, 0, 0),
			screen(false, false, 1440, 900),
		}, screenSize{1440, 900}},
		{"current with unknown size is skipped", []wailsruntime.Screen{
			screen(true, false, 0, 0),
			screen(false, true, 1920, 1080),
		}, screenSize{1920, 1080}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := pickScreen(tc.screens); got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestWindowStateRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "window.json")
	if got := loadWindowState(path); got != (windowState{}) {
		t.Fatalf("missing file should load as zero state, got %+v", got)
	}
	want := windowState{Width: 1400, Height: 800, Maximised: true}
	if err := saveWindowState(path, want); err != nil {
		t.Fatalf("save: %v", err)
	}
	if got := loadWindowState(path); got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	if err := os.WriteFile(path, []byte("{not json"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := loadWindowState(path); got != (windowState{}) {
		t.Fatalf("corrupt file should load as zero state, got %+v", got)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("corrupt file should have been quarantined, stat err = %v", err)
	}
}
