package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gossh "golang.org/x/crypto/ssh"
)

func testKey(t *testing.T) gossh.PublicKey {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key, err := gossh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func TestAddHostKeyAppendsWholeLines(t *testing.T) {
	fp := filepath.Join(t.TempDir(), "known_hosts")
	// A previous crash left a line without its newline.
	if err := os.WriteFile(fp, []byte("old.example ssh-ed25519 AAAA"), 0600); err != nil {
		t.Fatal(err)
	}
	s := &KnownHostStore{filePath: fp}
	if err := s.AddHostKey("new.example", 2222, testKey(t)); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := s.AddHostKey("other.example", 22, testKey(t)); err != nil {
		t.Fatalf("add second: %v", err)
	}
	data, _ := os.ReadFile(fp)
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("want 3 complete lines, got %q", string(data))
	}
	if !strings.HasPrefix(lines[1], "[new.example]:2222 ") || !strings.HasPrefix(lines[2], "other.example ") {
		t.Fatalf("unexpected lines: %q", lines)
	}
	entries, _ := os.ReadDir(filepath.Dir(fp))
	for _, e := range entries {
		if e.Name() != "known_hosts" {
			t.Fatalf("temp file left behind: %s", e.Name())
		}
	}
}
