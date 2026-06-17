package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mimir/dotenv"
)

func TestIsWSLTerminalType(t *testing.T) {
	cases := map[string]bool{
		"wsl": true, "WSL": true, "  wsl  ": true, "Wsl": true,
		"bash": false, "zsh": false, "ssh": false, "": false, "wsl2": false,
	}
	for input, want := range cases {
		if got := isWSLTerminalType(input); got != want {
			t.Errorf("isWSLTerminalType(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestInterpretEnvOutput(t *testing.T) {
	if _, err := interpretEnvOutput(envMissingSentinel + "\n"); err == nil {
		t.Error("expected error for missing sentinel, got nil")
	}
	content, err := interpretEnvOutput("FOO=bar\n")
	if err != nil || content != "FOO=bar\n" {
		t.Errorf("interpretEnvOutput passthrough = %q, %v", content, err)
	}
}

func TestReadDotEnvFile(t *testing.T) {
	dir := t.TempDir()

	t.Run("missing", func(t *testing.T) {
		if _, err := readDotEnvFile(filepath.Join(dir, ".env")); err == nil {
			t.Error("expected error for missing file")
		}
	})

	t.Run("regular", func(t *testing.T) {
		path := filepath.Join(dir, ".env")
		if err := os.WriteFile(path, []byte("FOO=bar\n"), 0600); err != nil {
			t.Fatal(err)
		}
		content, err := readDotEnvFile(path)
		if err != nil || content != "FOO=bar\n" {
			t.Errorf("readDotEnvFile = %q, %v", content, err)
		}
	})

	t.Run("directory is rejected", func(t *testing.T) {
		sub := filepath.Join(dir, "envdir")
		if err := os.Mkdir(sub, 0700); err != nil {
			t.Fatal(err)
		}
		if _, err := readDotEnvFile(sub); err == nil {
			t.Error("expected error for non-regular file")
		}
	})

	t.Run("oversized is capped", func(t *testing.T) {
		path := filepath.Join(dir, "big.env")
		big := strings.Repeat("A", dotenv.MaxFileSize+128)
		if err := os.WriteFile(path, []byte(big), 0600); err != nil {
			t.Fatal(err)
		}
		content, err := readDotEnvFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if len(content) != dotenv.MaxFileSize {
			t.Errorf("content length = %d, want cap %d", len(content), dotenv.MaxFileSize)
		}
	})
}

func TestMarshalDotEnv(t *testing.T) {
	out, err := marshalDotEnv("local", "/home/u/proj", "FOO=bar\n# c\nBAZ=qux\n")
	if err != nil {
		t.Fatal(err)
	}
	var result dotEnvResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatal(err)
	}
	if result.Source != "local" || result.Dir != "/home/u/proj" {
		t.Errorf("unexpected metadata: %+v", result)
	}
	want := []dotenv.Entry{{Key: "FOO", Value: "bar"}, {Key: "BAZ", Value: "qux"}}
	if len(result.Entries) != len(want) {
		t.Fatalf("entries = %+v, want %+v", result.Entries, want)
	}
	for i := range want {
		if result.Entries[i] != want[i] {
			t.Errorf("entry[%d] = %+v, want %+v", i, result.Entries[i], want[i])
		}
	}
}

func TestMarshalDotEnvEmptyIsNonNullArray(t *testing.T) {
	out, err := marshalDotEnv("remote", "", "# only comments\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"entries":[]`) {
		t.Errorf("expected empty entries array, got %s", out)
	}
}
