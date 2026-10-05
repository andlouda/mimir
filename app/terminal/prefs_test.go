package terminal

import "testing"

func TestShouldForceSimpleIM(t *testing.T) {
	cases := []struct {
		mode   string
		lang   string
		envSet bool
		want   bool
	}{
		{IMModuleAuto, "de_DE.UTF-8", false, true},
		{IMModuleAuto, "en_US.UTF-8", false, true},
		{IMModuleAuto, "ja_JP.UTF-8", false, false},
		{IMModuleAuto, "zh_CN.UTF-8", false, false},
		{IMModuleAuto, "ko_KR.UTF-8", false, false},
		{IMModuleAuto, "", false, true},
		{IMModuleSimple, "ja_JP.UTF-8", false, true},
		{IMModuleSystem, "de_DE.UTF-8", false, false},
		{IMModuleAuto, "de_DE.UTF-8", true, false}, // the user's own GTK_IM_MODULE wins
		{"garbage", "de_DE.UTF-8", false, true},
	}
	for _, c := range cases {
		if got := ShouldForceSimpleIM(c.mode, c.lang, c.envSet); got != c.want {
			t.Fatalf("mode=%s lang=%q env=%v: got %v want %v", c.mode, c.lang, c.envSet, got, c.want)
		}
	}
	if NormalizePromptMode("SHELL") != PromptModeShell || NormalizePromptMode("x") != PromptModeMimir {
		t.Fatalf("prompt mode normalisation")
	}
}

func TestPrefsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("APPDATA", dir)
	if PromptMode() != PromptModeMimir || IMModule() != IMModuleAuto {
		t.Fatalf("defaults")
	}
	if err := WritePref(PromptModeFile, PromptModeShell); err != nil {
		t.Fatal(err)
	}
	if err := WritePref(IMModuleFile, IMModuleSystem); err != nil {
		t.Fatal(err)
	}
	if PromptMode() != PromptModeShell || IMModule() != IMModuleSystem {
		t.Fatalf("persisted values not read back: %s %s", PromptMode(), IMModule())
	}
}
