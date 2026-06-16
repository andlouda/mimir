//go:build !linux

package main

// configureInputMethod is a no-op outside Linux; the WebKitGTK input-method
// workaround it applies is specific to the GTK webview.
func configureInputMethod() {}
