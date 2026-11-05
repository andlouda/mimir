package ssh

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListSSHKeysWithoutKeysIsEmptyNotNil(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0700); err != nil {
		t.Fatal(err)
	}
	// Only files that are skipped: no private key.
	for _, name := range []string{"known_hosts", "id_ed25519.pub"} {
		if err := os.WriteFile(filepath.Join(home, ".ssh", name), []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	keys, err := ListSSHKeys()
	if err != nil {
		t.Fatal(err)
	}
	if keys == nil {
		t.Fatal("keys must be an empty slice, not nil (encodes as null)")
	}
	if len(keys) != 0 {
		t.Fatalf("expected no keys, got %+v", keys)
	}
}
