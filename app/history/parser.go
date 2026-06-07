package history

import (
	"bytes"
	"encoding/base64"
	"strings"
)

// OSC 7337 format:
// \x1b]7337;cmd=BASE64;exit=CODE;cwd=PATH;host=HOST;user=USER;shell=SHELL;ts=ISO8601\x07

var oscPrefix = []byte("\x1b]7337;")

const oscTerminator = '\x07'

const (
	// maxOSCSeqLen bounds how many bytes we will buffer while waiting for the
	// BEL terminator of an OSC 7337 sequence. Terminal output is untrusted
	// (a remote host or a `cat`-ed file can emit escape sequences), so an
	// unterminated "sequence" must never grow the residual buffer without
	// bound. Beyond this limit the bytes are treated as ordinary output.
	maxOSCSeqLen = 16 * 1024
	// maxCmdLen caps the decoded command length. A real shell command line is
	// short; anything larger is rejected as spoofed/garbage.
	maxCmdLen = 8 * 1024
	// maxFieldLen caps metadata fields (cwd/host/user/shell/ts).
	maxFieldLen = 1024
)

// ParsedCommand holds data extracted from a single OSC 7337 sequence.
type ParsedCommand struct {
	Command   string
	ExitCode  string
	CWD       string
	Hostname  string
	Username  string
	Shell     string
	Timestamp string
}

// StripAndExtract scans data for OSC 7337 sequences, removes them from the
// output, and returns the cleaned data, any parsed commands, and any residual
// bytes (an incomplete sequence at the end that may complete with the next
// read). The residual is bounded by maxOSCSeqLen so untrusted output cannot
// cause unbounded memory growth across reads.
func StripAndExtract(data []byte) (cleaned []byte, commands []ParsedCommand, residual []byte) {
	if len(data) == 0 {
		return data, nil, nil
	}

	// Fast path: no ESC in data at all
	if !bytes.Contains(data, []byte{0x1b}) {
		return data, nil, nil
	}

	cleaned = make([]byte, 0, len(data))
	pos := 0

	for pos < len(data) {
		// Find next ESC byte
		idx := bytes.IndexByte(data[pos:], 0x1b)
		if idx == -1 {
			// No more ESC sequences — append rest and return
			cleaned = append(cleaned, data[pos:]...)
			return cleaned, commands, nil
		}

		// Append everything before the ESC
		cleaned = append(cleaned, data[pos:pos+idx]...)
		escPos := pos + idx

		// Check if we have enough bytes for the prefix
		remaining := data[escPos:]
		if len(remaining) < len(oscPrefix) {
			// Could be the start of our sequence — treat as residual, but only
			// while it can still plausibly become our prefix.
			if bytes.HasPrefix(oscPrefix, remaining) {
				return cleaned, commands, remaining
			}
			cleaned = append(cleaned, 0x1b)
			pos = escPos + 1
			continue
		}

		// Check if this is our prefix
		if !bytes.HasPrefix(remaining, oscPrefix) {
			// Not our sequence — pass through the ESC byte and continue
			cleaned = append(cleaned, 0x1b)
			pos = escPos + 1
			continue
		}

		// Found our prefix. Look for the terminator (BEL \x07).
		termIdx := bytes.IndexByte(remaining, oscTerminator)
		if termIdx == -1 {
			// Incomplete sequence. Only buffer it as residual while it is short
			// enough to be a real OSC 7337 sequence; otherwise it is untrusted
			// noise — emit the ESC literally and resume scanning after it.
			if len(remaining) > maxOSCSeqLen {
				cleaned = append(cleaned, 0x1b)
				pos = escPos + 1
				continue
			}
			return cleaned, commands, remaining
		}
		if termIdx > maxOSCSeqLen {
			// Terminator exists but the payload is implausibly large — reject.
			cleaned = append(cleaned, 0x1b)
			pos = escPos + 1
			continue
		}

		// Extract the payload between prefix and terminator
		payload := remaining[len(oscPrefix):termIdx]
		cmd := parseOSCPayload(payload)
		if cmd.Command != "" {
			commands = append(commands, cmd)
		}

		// Advance past the terminator
		pos = escPos + termIdx + 1
	}

	return cleaned, commands, nil
}

// parseOSCPayload parses key=value pairs separated by semicolons. All values
// are length-capped and stripped of control characters: the payload arrives
// over an untrusted channel (terminal output), so a command line containing
// embedded newlines/control bytes is rejected rather than stored, which would
// otherwise enable history poisoning and downstream prompt injection.
func parseOSCPayload(payload []byte) ParsedCommand {
	var cmd ParsedCommand
	parts := strings.Split(string(payload), ";")
	for _, part := range parts {
		eqIdx := strings.IndexByte(part, '=')
		if eqIdx < 0 {
			continue
		}
		key := part[:eqIdx]
		value := part[eqIdx+1:]

		switch key {
		case "cmd":
			decoded, err := base64.StdEncoding.DecodeString(value)
			if err != nil || len(decoded) > maxCmdLen {
				continue
			}
			c := string(decoded)
			// A legitimate command is a single line. Reject embedded control
			// characters (NUL, CR, LF) that indicate a forged sequence.
			if strings.ContainsAny(c, "\x00\r\n") {
				continue
			}
			cmd.Command = c
		case "exit":
			cmd.ExitCode = sanitizeField(value)
		case "cwd":
			cmd.CWD = sanitizeField(value)
		case "host":
			cmd.Hostname = sanitizeField(value)
		case "user":
			cmd.Username = sanitizeField(value)
		case "shell":
			cmd.Shell = sanitizeField(value)
		case "ts":
			cmd.Timestamp = sanitizeField(value)
		}
	}
	return cmd
}

// sanitizeField caps length and removes control characters from a metadata
// field value.
func sanitizeField(s string) string {
	if len(s) > maxFieldLen {
		s = s[:maxFieldLen]
	}
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
}
