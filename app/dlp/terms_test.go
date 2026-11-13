package dlp

import "testing"

func TestAddTermsMasksWholeWordsLongestFirst(t *testing.T) {
	m := NewMapping()
	text := "t3@pironman:~$ ssh db01.corp.example  # corp vpn, pironman2 is another box"
	added := m.AddTerms(KindHost, []string{"pironman", "db01.corp.example", "corp"}, text)
	added += m.AddTerms(KindUser, []string{"t3"}, text)
	if added != 3 {
		t.Fatalf("added = %d, want 3 (t3 is too short)", added)
	}
	got := m.Mask(text)
	want := "t3@MIMIR_HOST_2:~$ ssh MIMIR_HOST_1  # MIMIR_HOST_3 vpn, pironman2 is another box"
	if got != want {
		t.Fatalf("Mask:\n got %q\nwant %q", got, want)
	}
	back, report := m.Unmask(got)
	if back != text || len(report.Unknown) != 0 {
		t.Fatalf("round trip:\n got %q\nwant %q (%+v)", back, text, report)
	}
}

func TestAddTermsRejectsWhatIsNotInTheTextOrNotATerm(t *testing.T) {
	m := NewMapping()
	text := "deploy to alpha-prod as deployer on localhost"
	rejected := []string{
		"beta-prod",      // not in the text
		"prod",           // only as part of alpha-prod? no: glued by "-" is fine, see below
		"localhost",      // generic
		"to",             // too short
		"alpha prod",     // whitespace
		"MIMIR_HOST_1",   // a placeholder
		"12345",          // no letter
		"-alpha",         // does not start with a word character
		"deploy to alph", // whitespace
	}
	// "prod" does occur as a word ("alpha-prod": "-" is not a word char).
	if n := m.AddTerms(KindHost, rejected, text); n != 1 {
		t.Fatalf("added = %d, want only \"prod\"", n)
	}
	if got := m.Mask(text); got != "deploy to alpha-MIMIR_HOST_1 as deployer on localhost" {
		t.Fatalf("Mask = %q", got)
	}
}

func TestLearnedTermsStayMaskedInLaterTexts(t *testing.T) {
	m := NewMapping()
	first := "connect to pironman failed"
	m.AddTerms(KindHost, []string{"pironman"}, first)
	if got := m.Mask(first); got != "connect to MIMIR_HOST_1 failed" {
		t.Fatalf("first = %q", got)
	}
	// No detector call for the second text: the term is known.
	if got := m.Mask("retry pironman, then Pironman"); got != "retry MIMIR_HOST_1, then Pironman" {
		t.Fatalf("second = %q", got)
	}
}

func TestTermsNeverTouchPlaceholders(t *testing.T) {
	m := NewMapping()
	text := "host 10.0.3.17 MIMIR and EMAIL and HOST words"
	m.AddTerms(KindName, []string{"MIMIR", "EMAIL", "HOST"}, text)
	got := m.Mask(text)
	// "HOST" is a generic word and stays; the placeholder for the address
	// keeps its "MIMIR" and "IP" although "MIMIR" is a term now.
	want := "host MIMIR_IP_1 MIMIR_NAME_2 and MIMIR_NAME_1 and HOST words"
	if got != want {
		t.Fatalf("Mask:\n got %q\nwant %q", got, want)
	}
	back, _ := m.Unmask(got)
	if back != text {
		t.Fatalf("round trip = %q", back)
	}
}

func TestAddTermsIsBounded(t *testing.T) {
	m := NewMapping()
	text := ""
	var terms []string
	for i := 0; i < MaxTermsPerCall+10; i++ {
		term := "node" + string(rune('a'+i/26)) + string(rune('a'+i%26))
		terms = append(terms, term)
		text += term + " "
	}
	if n := m.AddTerms(KindHost, terms, text); n != MaxTermsPerCall {
		t.Fatalf("added = %d, want the cap %d", n, MaxTermsPerCall)
	}
}
