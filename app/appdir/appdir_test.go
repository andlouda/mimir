package appdir

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHomeDefaultsToTheUserConfigDir(t *testing.T) {
	t.Setenv(EnvHome, "")
	configDir, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("UserConfigDir: %v", err)
	}
	home, err := Home()
	if err != nil {
		t.Fatalf("Home: %v", err)
	}
	if home != filepath.Join(configDir, "mimir") {
		t.Fatalf("Home = %q, want %q", home, filepath.Join(configDir, "mimir"))
	}
	if Custom() {
		t.Fatal("Custom reported an override without MIMIR_HOME")
	}
}

func TestMimirHomeSelectsTheDirectoryDirectly(t *testing.T) {
	scratch := filepath.Join(t.TempDir(), "scratch")
	t.Setenv(EnvHome, "  "+scratch+"  ")
	home, err := Home()
	if err != nil {
		t.Fatalf("Home: %v", err)
	}
	if home != scratch {
		t.Fatalf("Home = %q, want %q (no \"mimir\" appended)", home, scratch)
	}
	if !Custom() {
		t.Fatal("Custom did not report the override")
	}
	if _, err := os.Stat(scratch); !os.IsNotExist(err) {
		t.Fatalf("Home created the directory (stat err %v)", err)
	}
}

func TestRelativeMimirHomeBecomesAbsolute(t *testing.T) {
	t.Setenv(EnvHome, "scratch-home")
	home, err := Home()
	if err != nil {
		t.Fatalf("Home: %v", err)
	}
	if !filepath.IsAbs(home) || filepath.Base(home) != "scratch-home" {
		t.Fatalf("Home = %q, want an absolute path ending in scratch-home", home)
	}
}
