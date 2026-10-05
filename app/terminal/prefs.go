package terminal

import (
	"os"
	"path/filepath"
	"strings"
)

// Shell preferences read by the launch paths (both the terminal package
// and the App layer use these files; the App layer writes them).

const (
	PromptModeFile = "prompt_mode"
	// PromptModeMimir sets Mimir's short prompt ("dir $ ") after the user's
	// rc file; PromptModeShell leaves the prompt alone (Starship, oh-my-posh,
	// a custom PS1 survive).
	PromptModeMimir = "mimir"
	PromptModeShell = "shell"

	IMModuleFile = "im_module"
	// IMModuleAuto forces GTK's simple input context (the dead-key / umlaut
	// workaround for WebKitGTK) unless the locale is a CJK/complex-script
	// one or the user already chose a module; IMModuleSimple always forces
	// it; IMModuleSystem never does.
	IMModuleAuto   = "auto"
	IMModuleSimple = "simple"
	IMModuleSystem = "system"
)

func prefPath(name string) (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "mimir", name), nil
}

func readPref(name, fallback string) string {
	path, err := prefPath(name)
	if err != nil {
		return fallback
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fallback
	}
	v := strings.TrimSpace(strings.ToLower(string(data)))
	if v == "" {
		return fallback
	}
	return v
}

// WritePref persists a preference value (0600) below the config dir.
func WritePref(name, value string) error {
	path, err := prefPath(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(value+"\n"), 0600)
}

func NormalizePromptMode(v string) string {
	if strings.TrimSpace(strings.ToLower(v)) == PromptModeShell {
		return PromptModeShell
	}
	return PromptModeMimir
}

// PromptMode returns the persisted prompt preference (default: mimir).
func PromptMode() string {
	return NormalizePromptMode(readPref(PromptModeFile, PromptModeMimir))
}

func NormalizeIMModule(v string) string {
	switch strings.TrimSpace(strings.ToLower(v)) {
	case IMModuleSimple:
		return IMModuleSimple
	case IMModuleSystem:
		return IMModuleSystem
	default:
		return IMModuleAuto
	}
}

// IMModule returns the persisted input-method preference (default: auto).
func IMModule() string {
	return NormalizeIMModule(readPref(IMModuleFile, IMModuleAuto))
}

// cjkLocale reports whether a locale needs a real input method (CJK and
// other complex scripts), in which case the simple context must not be
// forced because it would disable ibus/fcitx entirely.
func cjkLocale(lang string) bool {
	l := strings.ToLower(strings.TrimSpace(lang))
	for _, p := range []string{"zh", "ja", "ko", "vi", "th", "km", "lo", "my", "bo", "hi", "bn", "ta", "te", "kn", "ml", "si", "ar", "fa", "he"} {
		if strings.HasPrefix(l, p+"_") || strings.HasPrefix(l, p+".") || l == p {
			return true
		}
	}
	return false
}

// ShouldForceSimpleIM decides whether GTK_IM_MODULE=gtk-im-context-simple
// is set before GTK initialises. envSet is true when the user exported a
// module themselves (always respected); lang is LC_ALL / LC_CTYPE / LANG.
func ShouldForceSimpleIM(mode string, lang string, envSet bool) bool {
	if envSet {
		return false
	}
	switch NormalizeIMModule(mode) {
	case IMModuleSimple:
		return true
	case IMModuleSystem:
		return false
	default:
		return !cjkLocale(lang)
	}
}

// CurrentLocale returns the locale that decides the input method.
func CurrentLocale() string {
	for _, k := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}
