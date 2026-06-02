package update

import "testing"

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want int
	}{
		{name: "newer patch", a: "v0.1.2", b: "0.1.1", want: 1},
		{name: "same ignores v", a: "v1.2.3", b: "1.2.3", want: 0},
		{name: "older minor", a: "0.9.0", b: "1.0.0", want: -1},
		{name: "prerelease numbers", a: "1.2.4-dev", b: "1.2.3", want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := compareVersions(tt.a, tt.b); got != tt.want {
				t.Fatalf("compareVersions(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestValidRepository(t *testing.T) {
	if !validRepository("owner/repo") {
		t.Fatal("expected owner/repo to be valid")
	}
	if validRepository("owner/repo/extra") {
		t.Fatal("expected owner/repo/extra to be invalid")
	}
	if validRepository("owner repo") {
		t.Fatal("expected repository with whitespace to be invalid")
	}
}
