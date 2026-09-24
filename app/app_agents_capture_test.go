package main

import "testing"

func TestParseAgentCaptureAlternate(t *testing.T) {
	text, width, alt := parseAgentCapture("120 1\n__MIMIR_PS__\nline one\nline two\n")
	if width != 120 || !alt || text != "line one\nline two" {
		t.Fatalf("got %q %d %v", text, width, alt)
	}
	_, width, alt = parseAgentCapture("80 0\n__MIMIR_PS__\nx")
	if width != 80 || alt {
		t.Fatalf("main screen: %d %v", width, alt)
	}
	// Older head format (width only) still parses.
	_, width, alt = parseAgentCapture("100\n__MIMIR_PS__\nx")
	if width != 100 || alt {
		t.Fatalf("legacy head: %d %v", width, alt)
	}
}
