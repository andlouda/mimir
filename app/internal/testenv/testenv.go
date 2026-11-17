// Package testenv keeps test runs away from the developer's real Mimir data.
//
// Mimir resolves its data directory through os.UserConfigDir and
// os.UserHomeDir. Those read a different variable on every platform
// (APPDATA on Windows, HOME on macOS, XDG_CONFIG_HOME on Linux), so a test
// that redirects only one of them silently works on the real directory
// elsewhere: on Windows `go test` used to empty the saved session and
// overwrite the AI tool flow config of the installed app.
//
// Every test package therefore starts through Main, which points all of
// these variables at a throwaway directory before the first test runs. The
// guard in the root package (TestEveryTestPackageIsIsolated) fails when a
// package forgets to.
package testenv

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// redirected lists every variable that leads to a per-user directory, with
// the sub directory it gets inside the throwaway root.
var redirected = []struct{ name, sub string }{
	{"HOME", "home"},
	{"USERPROFILE", "home"},
	{"XDG_CONFIG_HOME", filepath.Join("home", ".config")},
	{"XDG_CACHE_HOME", filepath.Join("home", ".cache")},
	{"XDG_DATA_HOME", filepath.Join("home", ".local", "share")},
	{"XDG_STATE_HOME", filepath.Join("home", ".local", "state")},
	{"APPDATA", filepath.Join("home", "AppData", "Roaming")},
	{"LOCALAPPDATA", filepath.Join("home", "AppData", "Local")},
}

// Main runs the tests of a package inside an isolated user environment.
// Call it from TestMain:
//
//	func TestMain(m *testing.M) { testenv.Main(m) }
func Main(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	root, err := os.MkdirTemp("", "mimir-test-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "testenv: no temp dir, refusing to run on real user data: %v\n", err)
		return 1
	}
	defer os.RemoveAll(root)

	if err := Isolate(root); err != nil {
		fmt.Fprintf(os.Stderr, "testenv: %v\n", err)
		return 1
	}
	return m.Run()
}

// Isolate points every per-user directory variable below root and forces the
// file based secret store, so tests never reach the OS keyring.
func Isolate(root string) error {
	for _, v := range redirected {
		dir := filepath.Join(root, v.sub)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
		if err := os.Setenv(v.name, dir); err != nil {
			return fmt.Errorf("set %s: %w", v.name, err)
		}
	}
	if err := os.Setenv("MIMIR_SECRET_BACKEND", "file"); err != nil {
		return fmt.Errorf("set MIMIR_SECRET_BACKEND: %w", err)
	}
	// An explicit config path from the developer's shell would bypass the
	// redirected directories.
	if err := os.Unsetenv("MIMIR_AI_TOOL_FLOW_CONFIG"); err != nil {
		return fmt.Errorf("unset MIMIR_AI_TOOL_FLOW_CONFIG: %w", err)
	}
	return nil
}
