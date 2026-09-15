package update

import "testing"

func TestValidateAssetURL(t *testing.T) {
	allowed := []string{
		"https://github.com/owner/repo/releases/download/v1.0.0/mimir-linux-amd64.tar.gz",
		"https://objects.githubusercontent.com/github-production-release-asset/abc",
		"https://release-assets.githubusercontent.com/abc",
	}
	for _, u := range allowed {
		if err := validateAssetURL(u); err != nil {
			t.Fatalf("expected %s to be allowed, got %v", u, err)
		}
	}

	denied := []string{
		"http://github.com/owner/repo/releases/download/v1.0.0/mimir.tar.gz",
		"https://evil.example.com/mimir.tar.gz",
		"https://github.com.evil.example.com/x",
		"https://notgithub.com/x",
		"ftp://github.com/x",
		"",
	}
	for _, u := range denied {
		if err := validateAssetURL(u); err == nil {
			t.Fatalf("expected %q to be rejected", u)
		}
	}
}
