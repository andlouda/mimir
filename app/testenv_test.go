package main

import (
	"testing"

	mimirssh "mimir/ssh"
)

// isolateUserDirs points every per-user config/cache location at fresh
// temp dirs, so stores created by a test never share state with other
// tests or the machine. os.UserConfigDir reads XDG_CONFIG_HOME on Linux,
// HOME on macOS and APPDATA on Windows; all are set.
func isolateUserDirs(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("LOCALAPPDATA", t.TempDir())
}

// profileIDByName finds the profile a test created, wherever the store
// lists it.
func profileIDByName(t *testing.T, profiles []mimirssh.Profile, name string) string {
	t.Helper()
	for _, p := range profiles {
		if p.Name == name {
			return p.ID
		}
	}
	t.Fatalf("profile %q not in %d profiles", name, len(profiles))
	return ""
}
