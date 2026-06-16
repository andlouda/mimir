package dotenv

import "testing"

func TestParseBasic(t *testing.T) {
	content := "# comment\nFOO=bar\n\nexport BAZ=qux\nEMPTY=\n"
	entries := Parse(content)
	want := []Entry{
		{"FOO", "bar"},
		{"BAZ", "qux"},
		{"EMPTY", ""},
	}
	assertEntries(t, entries, want)
}

func TestParseQuoting(t *testing.T) {
	content := `DQ="hello world"
SQ='raw $value #notcomment'
ESC="line1\nline2\t\"q\""
INLINE=value # trailing comment
HASHINVALUE=ab#cd
WS=  spaced  ` + "\n"
	entries := Parse(content)
	want := []Entry{
		{"DQ", "hello world"},
		{"SQ", "raw $value #notcomment"},
		{"ESC", "line1\nline2\t\"q\""},
		{"INLINE", "value"},
		{"HASHINVALUE", "ab#cd"}, // no space before # → not a comment
		{"WS", "spaced"},
	}
	assertEntries(t, entries, want)
}

func TestParseSkipsMalformed(t *testing.T) {
	content := "GOOD=1\nno equals here\n123BAD=x\nBAD KEY=y\n.LEADINGDOT=z\nVALID_2=ok\n"
	entries := Parse(content)
	want := []Entry{
		{"GOOD", "1"},
		{"VALID_2", "ok"},
	}
	assertEntries(t, entries, want)
}

func TestParseCRLFAndBOM(t *testing.T) {
	content := "\ufeffFOO=bar\r\nBAZ=qux\r\n"
	entries := Parse(content)
	assertEntries(t, entries, []Entry{{"FOO", "bar"}, {"BAZ", "qux"}})
}

func TestParseDottedKey(t *testing.T) {
	entries := Parse("app.name=mimir\n")
	assertEntries(t, entries, []Entry{{"app.name", "mimir"}})
}

func assertEntries(t *testing.T, got, want []Entry) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("entry count = %d, want %d (%+v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}
