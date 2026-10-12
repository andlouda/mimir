package safeio

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.json")
	type doc struct{ A int }

	v := doc{A: 7}
	if err := LoadJSON(path, &v); err != nil || v.A != 7 {
		t.Fatalf("missing file: err=%v v=%+v", err, v)
	}

	if err := os.WriteFile(path, []byte(`{"A": 3}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := LoadJSON(path, &v); err != nil || v.A != 3 {
		t.Fatalf("valid file: err=%v v=%+v", err, v)
	}

	if err := os.WriteFile(path, []byte(`{"A": `), 0600); err != nil {
		t.Fatal(err)
	}
	v = doc{A: 9}
	err := LoadJSON(path, &v)
	if !errors.Is(err, ErrCorrupt) {
		t.Fatalf("corrupt file: want ErrCorrupt, got %v", err)
	}
	if v.A != 9 {
		t.Fatalf("corrupt file must leave v untouched, got %+v", v)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("corrupt file must be moved aside, stat: %v", statErr)
	}
	aside, _ := filepath.Glob(path + ".corrupt-*")
	if len(aside) != 1 {
		t.Fatalf("expected one .corrupt copy, got %v", aside)
	}
	if data, _ := os.ReadFile(aside[0]); string(data) != `{"A": ` {
		t.Fatalf("the copy must keep the broken content, got %q", data)
	}
}
