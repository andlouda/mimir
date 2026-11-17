package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEveryTestPackageIsIsolated fails when a package has tests but does not
// start them through testenv.Main. Without it a test resolves the data
// directory of the installed app and overwrites the developer's session,
// settings and audit logs (see internal/testenv).
func TestEveryTestPackageIsIsolated(t *testing.T) {
	skip := map[string]bool{"node_modules": true, "frontend": true, "build": true, ".git": true}
	withTests := map[string]bool{}
	isolated := map[string]bool{}

	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skip[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		dir := filepath.Dir(path)
		withTests[dir] = true
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(data), "testenv.Main(m)") {
			isolated[dir] = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if len(withTests) == 0 {
		t.Fatal("found no test packages; the guard is looking in the wrong place")
	}
	for dir := range withTests {
		if !isolated[dir] {
			t.Errorf("package %s has tests but no `func TestMain(m *testing.M) { testenv.Main(m) }`", dir)
		}
	}
}

// TestUserDirectoriesAreRedirected proves the isolation is in effect for this
// process: the config directory must not be the real one.
func TestUserDirectoriesAreRedirected(t *testing.T) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("UserConfigDir: %v", err)
	}
	if !strings.Contains(configDir, "mimir-test-") {
		t.Fatalf("config dir %q is not inside the test sandbox", configDir)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	if !strings.Contains(home, "mimir-test-") {
		t.Fatalf("home dir %q is not inside the test sandbox", home)
	}
}
