package main

import (
	"errors"
	"strings"
	"testing"
)

func enableLocalMasking(t *testing.T, app *App) {
	t.Helper()
	if _, err := app.UpdateDLPSettingsJSON(`{"localModelEnabled":true,"baseUrl":"http://127.0.0.1:11434/api/chat","model":"tiny"}`); err != nil {
		t.Fatalf("UpdateDLPSettingsJSON: %v", err)
	}
}

func TestLocalModelTermsAreMaskedForTheAgentAndTranslatedBack(t *testing.T) {
	freshConfigDir(t)
	var seen []string
	app := &App{termDetectorCaller: func(_ DLPSettings, prompt string) (string, error) {
		seen = append(seen, prompt)
		return "Here you go:\n{\"hosts\":[\"pironman\",\"notinthetext\"],\"users\":[\"deployer\"],\"names\":[\"Acme\"]}", nil
	}}
	enableLocalMasking(t, app)

	out := maskForAgent(t, app, 7, "deployer@pironman:~$ curl 10.0.3.17  # Acme staging\nAPI_TOKEN=abc123verysecret")
	want := "MIMIR_USER_1@MIMIR_HOST_1:~$ curl MIMIR_IP_1  # MIMIR_NAME_1 staging\n[REDACTED]"
	if out.Text != want {
		t.Fatalf("masked:\n got %q\nwant %q", out.Text, want)
	}
	if out.Masked["HOST"] != 1 || out.Masked["USER"] != 1 || out.Masked["NAME"] != 1 || out.Masked["IP"] != 1 {
		t.Fatalf("counts = %v", out.Masked)
	}

	// The local model saw the text after the pattern stage: no secret, no
	// address.
	if len(seen) != 1 || strings.Contains(seen[0], "abc123verysecret") || strings.Contains(seen[0], "10.0.3.17") {
		t.Fatalf("detector prompt carries unmasked data: %q", seen)
	}

	back := unmaskFromAgent(t, app, 7, "ssh MIMIR_USER_1@MIMIR_HOST_1 'curl -s MIMIR_IP_1'")
	if back.Text != "ssh deployer@pironman 'curl -s 10.0.3.17'" || len(back.Unknown) != 0 {
		t.Fatalf("back = %#v", back)
	}
}

func TestLocalModelFailureSendsNothing(t *testing.T) {
	for name, caller := range map[string]func(DLPSettings, string) (string, error){
		"unreachable": func(DLPSettings, string) (string, error) { return "", errors.New("connection refused") },
		"not json":    func(DLPSettings, string) (string, error) { return "I cannot help with that.", nil },
		"wrong shape": func(DLPSettings, string) (string, error) { return `{"hosts":"pironman"}`, nil },
	} {
		t.Run(name, func(t *testing.T) {
			freshConfigDir(t)
			app := &App{termDetectorCaller: caller}
			enableLocalMasking(t, app)

			if raw, err := app.MaskTextForAgentJSON(7, "host pironman 10.0.3.17"); err == nil {
				t.Fatalf("masked send went through without the local model: %s", raw)
			}

			// The built-in AI path must not reach the provider either.
			called := false
			app.aiSettings = AISettings{Provider: aiProviderOpenAI, Model: "m", BaseURL: "unused", APIKey: "k"}
			app.aiProviderCaller = func(AISettings, string) (string, error) { called = true; return "ls", nil }
			if _, err := app.AskAI("suggest_next_command", "", "bash", "Bash", "host pironman 10.0.3.17"); err == nil {
				t.Fatal("AskAI went through without the local model")
			}
			if called {
				t.Fatal("the cloud provider was called although the local stage failed")
			}
		})
	}
}

func TestLocalModelStageIsOffByDefault(t *testing.T) {
	freshConfigDir(t)
	app := &App{termDetectorCaller: func(DLPSettings, string) (string, error) {
		t.Fatal("detector called although the stage is off")
		return "", nil
	}}
	out := maskForAgent(t, app, 7, "host pironman 10.0.3.17")
	if out.Text != "host pironman MIMIR_IP_1" {
		t.Fatalf("masked = %q", out.Text)
	}
}

func TestAskAIHidesTermsTheLocalModelNamed(t *testing.T) {
	freshConfigDir(t)
	var sent string
	app := maskTestApp(aiProviderOpenAI, func(input string) (string, error) {
		sent = input
		return "ssh MIMIR_HOST_1", nil
	})
	app.termDetectorCaller = func(DLPSettings, string) (string, error) {
		return `{"hosts":["pironman"],"users":[],"names":[]}`, nil
	}
	enableLocalMasking(t, app)

	result := askText(t, app, "write_command_from_goal", "log in to pironman", "ping pironman: unknown host")
	if strings.Contains(sent, "pironman") {
		t.Fatalf("host name left the machine: %s", sent)
	}
	if result.Text != "ssh pironman" || result.Warning != "" {
		t.Fatalf("result = %#v", result)
	}
}

func TestLocalModelURLMustBeLocal(t *testing.T) {
	for _, ok := range []string{"http://localhost:11434/api/chat", "http://127.0.0.1:11434/api/chat", "http://192.168.1.20:11434/api/chat", "https://10.0.0.5/api/chat", "http://[::1]:11434/api/chat"} {
		if err := validateLocalModelURL(ok); err != nil {
			t.Errorf("%s rejected: %v", ok, err)
		}
	}
	for _, bad := range []string{"https://8.8.8.8/api/chat", "ftp://localhost/x", "http:///api/chat", "https://203.0.113.9:11434/api/chat"} {
		if err := validateLocalModelURL(bad); err == nil {
			t.Errorf("%s accepted", bad)
		}
	}
	freshConfigDir(t)
	app := &App{}
	if _, err := app.UpdateDLPSettingsJSON(`{"localModelEnabled":true,"baseUrl":"https://8.8.8.8/api/chat","model":"m"}`); err == nil {
		t.Fatal("settings with a public endpoint were saved")
	}
}

func TestLongTextIsRefusedRatherThanCheckedInPart(t *testing.T) {
	freshConfigDir(t)
	calls := 0
	app := &App{termDetectorCaller: func(DLPSettings, string) (string, error) {
		calls++
		return `{"hosts":[],"users":[],"names":[]}`, nil
	}}
	enableLocalMasking(t, app)

	line := strings.Repeat("x", 99) + "\n"
	fits := strings.Repeat(line, termDetectorChunkChars*3/len(line))
	if _, err := app.MaskTextForAgentJSON(7, fits); err != nil {
		t.Fatalf("text of %d chars refused: %v", len(fits), err)
	}
	if calls < 3 {
		t.Fatalf("calls = %d, want the text split over several detector calls", calls)
	}
	tooLong := strings.Repeat(line, termDetectorChunkChars*(termDetectorMaxChunks+1)/len(line))
	if _, err := app.MaskTextForAgentJSON(7, tooLong); err == nil {
		t.Fatal("over-long text was sent")
	}
}
