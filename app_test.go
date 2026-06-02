package main

import (
	"runtime"
	"testing"
)

func TestIsValidPath(t *testing.T) {
	tests := []struct {
		name  string
		path  string
		valid bool
	}{
		{"empty path", "", false},
		{"relative path", "some/relative/path", false},
		{"path traversal with ..", "../etc/passwd", false},
		{"path traversal mid-path", "/home/user/../etc/passwd", false},
		{"windows path traversal", "C:\\Users\\..\\System32", false},
		{"absolute linux path", "/home/user/documents", true},
		{"linux root", "/", true},
		{"absolute windows path", "C:\\Users\\test", runtime.GOOS == "windows"},
		{"dot only", ".", false},
		{"double dot only", "..", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidPath(tt.path)
			if result != tt.valid {
				t.Errorf("isValidPath(%q) = %v, want %v", tt.path, result, tt.valid)
			}
		})
	}
}
