package main

import (
	"testing"
	"time"
)

func TestRateLimiterAllowsBurst(t *testing.T) {
	rl := newRateLimiter(time.Hour, 3)

	for i := 0; i < 3; i++ {
		if err := rl.allow("test"); err != nil {
			t.Fatalf("call %d should be allowed: %v", i, err)
		}
	}

	if err := rl.allow("test"); err == nil {
		t.Fatal("4th call should be denied")
	}
}

func TestRateLimiterResetsAfterInterval(t *testing.T) {
	rl := newRateLimiter(10*time.Millisecond, 1)

	if err := rl.allow("x"); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if err := rl.allow("x"); err == nil {
		t.Fatal("second call should be denied")
	}

	time.Sleep(15 * time.Millisecond)

	if err := rl.allow("x"); err != nil {
		t.Fatalf("after interval: %v", err)
	}
}

func TestRateLimiterSeparateKeys(t *testing.T) {
	rl := newRateLimiter(time.Hour, 1)

	if err := rl.allow("a"); err != nil {
		t.Fatal(err)
	}
	if err := rl.allow("b"); err != nil {
		t.Fatal(err)
	}
	if err := rl.allow("a"); err == nil {
		t.Fatal("key 'a' should be exhausted")
	}
}

func TestIsValidTmuxSessionName(t *testing.T) {
	valid := []string{"mimir-ssh-abc-1", "test_session", "ABC123"}
	for _, name := range valid {
		if !isValidTmuxSessionName(name) {
			t.Errorf("%q should be valid", name)
		}
	}

	invalid := []string{"", "foo bar", "a;b", "../evil", "x\x00y", "a/b"}
	for _, name := range invalid {
		if isValidTmuxSessionName(name) {
			t.Errorf("%q should be invalid", name)
		}
	}
}
