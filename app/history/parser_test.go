package history

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

func TestStripAndExtractSplitOSCSequence(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte("echo hello"))
	first := []byte("before \x1b]7337;cmd=" + encoded[:6])
	second := []byte(encoded[6:] + ";exit=7;cwd=/tmp;host=dev;user=t3;shell=bash;ts=2026-05-28T10:00:00Z\x07 after")

	cleaned, commands, residual := StripAndExtract(first)
	if string(cleaned) != "before " {
		t.Fatalf("unexpected first cleaned output: %q", cleaned)
	}
	if len(commands) != 0 {
		t.Fatalf("expected no commands from incomplete sequence, got %d", len(commands))
	}

	combined := append(append([]byte(nil), residual...), second...)
	cleaned, commands, residual = StripAndExtract(combined)
	if len(residual) != 0 {
		t.Fatalf("expected no residual after completed sequence, got %q", residual)
	}
	if string(cleaned) != " after" {
		t.Fatalf("unexpected second cleaned output: %q", cleaned)
	}
	if len(commands) != 1 {
		t.Fatalf("expected one parsed command, got %d", len(commands))
	}
	if commands[0].Command != "echo hello" || commands[0].ExitCode != "7" || commands[0].CWD != "/tmp" {
		t.Fatalf("unexpected command: %+v", commands[0])
	}
}

// TestUnterminatedSequenceDoesNotGrowResidualUnbounded ensures a malicious,
// never-terminated OSC sequence (e.g. from a compromised remote host) is not
// buffered indefinitely.
func TestUnterminatedSequenceDoesNotGrowResidualUnbounded(t *testing.T) {
	data := append([]byte("\x1b]7337;cmd="), bytes.Repeat([]byte("A"), maxOSCSeqLen+5000)...)

	_, commands, residual := StripAndExtract(data)
	if len(residual) > maxOSCSeqLen {
		t.Fatalf("residual grew unbounded: %d bytes", len(residual))
	}
	if len(commands) != 0 {
		t.Fatalf("expected no commands from unterminated sequence, got %d", len(commands))
	}
}

// TestRejectsOversizedTerminatedPayload ensures a terminated but implausibly
// large payload is rejected rather than parsed/stored.
func TestRejectsOversizedTerminatedPayload(t *testing.T) {
	huge := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte("A"), maxOSCSeqLen))
	data := []byte("\x1b]7337;cmd=" + huge + ";exit=0\x07")

	_, commands, _ := StripAndExtract(data)
	if len(commands) != 0 {
		t.Fatalf("expected oversized payload to be rejected, got %d commands", len(commands))
	}
}

// TestRejectsControlCharsInCommand ensures a forged command line containing
// embedded newlines/control bytes is not recorded (history-poisoning /
// prompt-injection defense).
func TestRejectsControlCharsInCommand(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte("legit\nrm -rf /\n# injected"))
	data := []byte("\x1b]7337;cmd=" + encoded + ";exit=0;cwd=/tmp\x07")

	_, commands, _ := StripAndExtract(data)
	if len(commands) != 0 {
		t.Fatalf("expected command with control chars to be rejected, got %+v", commands)
	}
}

// TestRejectsOversizedDecodedCommand ensures a single huge command is dropped.
func TestRejectsOversizedDecodedCommand(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte("x"), maxCmdLen+1))
	data := []byte("\x1b]7337;cmd=" + encoded + ";exit=0\x07")

	_, commands, _ := StripAndExtract(data)
	if len(commands) != 0 {
		t.Fatalf("expected oversized command to be rejected, got %d", len(commands))
	}
}

// TestSanitizesMetadataFields ensures control characters are stripped from
// spoofable metadata fields.
func TestSanitizesMetadataFields(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte("echo hi"))
	// Use a non-BEL control char (TAB); a BEL would terminate the sequence early.
	data := []byte("\x1b]7337;cmd=" + encoded + ";exit=0;host=ev\til;cwd=/tmp\x07")

	_, commands, _ := StripAndExtract(data)
	if len(commands) != 1 {
		t.Fatalf("expected one command, got %d", len(commands))
	}
	if strings.ContainsAny(commands[0].Hostname, "\t\x00\r\n") {
		t.Fatalf("hostname not sanitized: %q", commands[0].Hostname)
	}
}
