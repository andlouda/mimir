package recording

import (
	"strings"
	"testing"
)

func TestScrubOutputRedactsKnownSecretFormats(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"openai", "key sk-abcdefghij1234567890ABCD here"},
		{"bearer", "Authorization: Bearer abcDEF1234567890.-_/+ token"},
		{"aws_access", "id AKIAIOSFODNN7EXAMPLE end"},
		{"github_classic", "ghp_" + strings.Repeat("a", 36)},
		{"github_pat", "github_pat_" + strings.Repeat("b", 30)},
		{"jwt", "tok eyJhbGciOiJIUzI1.eyJzdWIiOiIxMjM0.SflKxwRJSMeKKF2 end"},
		{"slack", "xoxb-123456789012-ABCDEFghijkl"},
		{"google_api", "AIzaSyA1234567890abcdefghijklmnopqrstuv"},
		{"stripe", "sk_live_abcdef1234567890ABCDEF"},
		{"password_kv", "DB_PASSWORD=hunter2supersecret"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ScrubOutput(tc.input)
			if !strings.Contains(got, "[REDACTED]") {
				t.Fatalf("expected redaction for %s, got %q", tc.name, got)
			}
		})
	}
}

func TestScrubOutputLeavesOrdinaryTextIntact(t *testing.T) {
	in := "total 24\ndrwxr-xr-x 2 user user 4096 May 31 10:00 docs\n$ ls -la"
	if got := ScrubOutput(in); got != in {
		t.Fatalf("ordinary text was altered:\n in: %q\nout: %q", in, got)
	}
}
