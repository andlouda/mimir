package main

import (
	"os"

	"mimir/terminal"
)

// configureInputMethod works around dropped/doubled dead-key and umlaut input
// in the WebKitGTK webview.
//
// With the default IBus/fcitx GTK input-method module, composed characters
// (ä, ö, ü, ß and dead-key accents) are delivered to the webview through GTK
// composition events. xterm.js then has two code paths that can emit the
// character — its compositionend handler and its keypress/input handler — and
// its dedup heuristic misfires under WebKitGTK, so the character is either
// dropped entirely or sent twice. Forcing the simple GTK input-method context
// routes these keys through plain key events, which xterm.js handles
// correctly, while still supporting compose-key and dead-key sequences for
// Latin scripts.
//
// Forcing the simple context also disables ibus/fcitx, which CJK and other
// complex-script users need even when they never exported GTK_IM_MODULE
// (GNOME sets it up without the variable). So the workaround is applied
// only for Latin-script locales by default, can be forced or switched off
// in Settings (terminal.IMModule), and never overrides a module the user
// exported. Must run before GTK/WebKit initialize (i.e. before wails.Run).
func configureInputMethod() {
	_, envSet := os.LookupEnv("GTK_IM_MODULE")
	if terminal.ShouldForceSimpleIM(terminal.IMModule(), terminal.CurrentLocale(), envSet) {
		_ = os.Setenv("GTK_IM_MODULE", "gtk-im-context-simple")
	}
}
