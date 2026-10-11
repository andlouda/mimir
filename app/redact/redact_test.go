package redact

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStringPatterns(t *testing.T) {
	cases := map[string]string{
		`curl -H "Authorization: Bearer abcdefghijklmnop" https://x`:          `curl -H "Authorization: Bearer [REDACTED]" https://x`,
		`export OPENAI_API_KEY=sk-abcdefghijklmnopqrstuvwxyz`:                 `export OPENAI_API_KEY=[REDACTED]`,
		`AWS_ACCESS_KEY_ID=AKIAABCDEFGHIJKLMNOP`:                              `AWS_ACCESS_KEY_ID=[REDACTED]`,
		`git clone https://ghp_abcdefghijklmnopqrstuvwxyz1234@github.com/x/y`: `git clone https://[REDACTED]@github.com/x/y`,
		`mysql -u root --password=hunter22 db`:                                `mysql -u root --password=[REDACTED] db`,
		`ls -la /tmp`:                                                         `ls -la /tmp`,
	}
	for in, want := range cases {
		got, _ := String(in)
		if got != want {
			t.Errorf("String(%q)\n got %q\nwant %q", in, got, want)
		}
	}
	if _, changed := String("nothing here"); changed {
		t.Fatal("unchanged text must report false")
	}
	pem := "-----BEGIN RSA PRIVATE KEY-----\nMIIE...\n-----END RSA PRIVATE KEY-----"
	if got, _ := String("key: " + pem); got != "key: [REDACTED]" {
		t.Fatalf("pem: %q", got)
	}
}

func TestValueKeysAndLimits(t *testing.T) {
	raw := json.RawMessage(`{"command":"echo ok","api_key":"x","nested":{"password":"p","list":[1,2,3,4]},"content":"` + strings.Repeat("a", 100) + `","prompt":"secret question"}`)
	out, changed := JSON(raw, Options{MaxString: 20, MaxItems: 3, MaxDepth: 3, Drop: []string{"prompt"}, KeepLong: []string{"command"}, MaxLong: 4096})
	if !changed {
		t.Fatal("must report a change")
	}
	var v map[string]any
	if err := json.Unmarshal(out, &v); err != nil {
		t.Fatal(err)
	}
	if v["api_key"] != Placeholder || v["nested"].(map[string]any)["password"] != Placeholder {
		t.Fatalf("sensitive keys not redacted: %s", out)
	}
	if _, ok := v["prompt"]; ok {
		t.Fatalf("dropped key still present: %s", out)
	}
	if c := v["content"].(string); !strings.HasSuffix(c, "...[truncated]") || len(c) > 20+len("...[truncated]") {
		t.Fatalf("content not capped: %q", c)
	}
	if l := v["nested"].(map[string]any)["list"].([]any); len(l) != 3 {
		t.Fatalf("list not capped: %v", l)
	}
	if v["command"] != "echo ok" {
		t.Fatalf("command changed: %v", v["command"])
	}
	// Invalid JSON never leaks through.
	if out, _ := JSON(json.RawMessage(`{bad`), Options{}); string(out) != `"[REDACTED]"` {
		t.Fatalf("invalid json: %s", out)
	}
	if out, changed := JSON(nil, Options{}); out != nil || changed {
		t.Fatal("empty input must pass through")
	}
}
