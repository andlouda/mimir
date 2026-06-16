package main

import "os"

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
// We only set it when the user has not chosen an input method themselves, so
// anyone relying on IBus/fcitx for CJK or other complex scripts keeps it.
// Must run before GTK/WebKit initialize (i.e. before wails.Run).
func configureInputMethod() {
	if _, ok := os.LookupEnv("GTK_IM_MODULE"); !ok {
		_ = os.Setenv("GTK_IM_MODULE", "gtk-im-context-simple")
	}
}
