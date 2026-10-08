package main

import "testing"

func TestParseTmuxPanePIDs(t *testing.T) {
	got := parseTmuxPanePIDs("mimir-1\t4242\nmimir-1\t9999\nmimir-2\t77\nbroken\n")
	if len(got) != 2 || got["mimir-1"] != 4242 || got["mimir-2"] != 77 {
		t.Fatalf("got %v", got)
	}
}
