package update

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"
)

const githubAPIBase = "https://api.github.com/repos"

// Asset describes a GitHub release asset relevant for updates.
type Asset struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256,omitempty"`
	IsChecksum  bool   `json:"isChecksum"`
	IsPlatform  bool   `json:"isPlatform"`
	ContentType string `json:"contentType,omitempty"`
}

// Info is the app-facing update check result.
type Info struct {
	Configured       bool    `json:"configured"`
	CurrentVersion   string  `json:"currentVersion"`
	LatestVersion    string  `json:"latestVersion"`
	UpdateAvailable  bool    `json:"updateAvailable"`
	Prerelease       bool    `json:"prerelease"`
	ReleaseURL       string  `json:"releaseUrl"`
	ReleaseName      string  `json:"releaseName"`
	PublishedAt      string  `json:"publishedAt"`
	Body             string  `json:"body"`
	Platform         string  `json:"platform"`
	ExecutablePath   string  `json:"executablePath,omitempty"`
	PlatformAsset    *Asset  `json:"platformAsset,omitempty"`
	ChecksumAsset    *Asset  `json:"checksumAsset,omitempty"`
	Assets           []Asset `json:"assets"`
	Error            string  `json:"error,omitempty"`
	Repository       string  `json:"repository,omitempty"`
	ManualUpdateOnly bool    `json:"manualUpdateOnly"`
	ExpectedSHA256   string  `json:"expectedSHA256,omitempty"`
}

type githubRelease struct {
	TagName     string        `json:"tag_name"`
	Name        string        `json:"name"`
	HTMLURL     string        `json:"html_url"`
	Prerelease  bool          `json:"prerelease"`
	PublishedAt string        `json:"published_at"`
	Body        string        `json:"body"`
	Assets      []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
	Digest             string `json:"digest"`
	ContentType        string `json:"content_type"`
}

// CheckGitHubRelease checks the latest GitHub release for a configured repository.
func CheckGitHubRelease(ctx context.Context, repository string, currentVersion string) (Info, error) {
	repository = strings.TrimSpace(repository)
	info := Info{
		Configured:       repository != "",
		CurrentVersion:   normalizeVersion(currentVersion),
		Platform:         runtime.GOOS + "/" + runtime.GOARCH,
		Repository:       repository,
		ManualUpdateOnly: true,
	}
	if repository == "" {
		info.Error = "update repository is not configured"
		return info, nil
	}
	if !validRepository(repository) {
		return info, fmt.Errorf("invalid update repository: %s", repository)
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubAPIBase+"/"+repository+"/releases/latest", nil)
	if err != nil {
		return info, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "mimir-update-checker")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return info, fmt.Errorf("check updates: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return info, fmt.Errorf("check updates: GitHub returned HTTP %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return info, fmt.Errorf("decode update response: %w", err)
	}

	info.LatestVersion = normalizeVersion(release.TagName)
	info.ReleaseName = release.Name
	info.ReleaseURL = release.HTMLURL
	info.Prerelease = release.Prerelease
	info.PublishedAt = release.PublishedAt
	info.Body = release.Body
	info.UpdateAvailable = compareVersions(info.LatestVersion, info.CurrentVersion) > 0
	info.Assets = mapAssets(release.Assets)

	for i := range info.Assets {
		if info.Assets[i].IsPlatform && info.PlatformAsset == nil {
			info.PlatformAsset = &info.Assets[i]
		}
		if info.Assets[i].IsChecksum && info.ChecksumAsset == nil {
			info.ChecksumAsset = &info.Assets[i]
		}
	}

	if info.PlatformAsset != nil {
		info.ManualUpdateOnly = false
	}

	if info.PlatformAsset != nil && info.ChecksumAsset != nil {
		if sha, err := resolveExpectedSHA(ctx, info); err == nil {
			info.ExpectedSHA256 = sha
		}
	}

	return info, nil
}

func validRepository(repository string) bool {
	parts := strings.Split(repository, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return false
	}
	return !strings.ContainsAny(repository, " \t\r\n")
}

func mapAssets(assets []githubAsset) []Asset {
	out := make([]Asset, 0, len(assets))
	for _, asset := range assets {
		sha := strings.TrimPrefix(asset.Digest, "sha256:")
		next := Asset{
			Name:        asset.Name,
			URL:         asset.BrowserDownloadURL,
			Size:        asset.Size,
			SHA256:      sha,
			IsChecksum:  isChecksumAsset(asset.Name),
			IsPlatform:  isPlatformAsset(asset.Name),
			ContentType: asset.ContentType,
		}
		out = append(out, next)
	}
	return out
}

func isChecksumAsset(name string) bool {
	lower := strings.ToLower(name)
	return lower == "checksums.txt" || strings.HasSuffix(lower, ".sha256") || strings.Contains(lower, "checksum")
}

func isPlatformAsset(name string) bool {
	lower := strings.ToLower(name)
	goos := runtime.GOOS

	switch goos {
	case "windows":
		return strings.Contains(lower, "windows") && assetArchMatches(lower)
	case "linux":
		return strings.Contains(lower, "linux") && assetArchMatches(lower)
	case "darwin":
		return (strings.Contains(lower, "darwin") || strings.Contains(lower, "macos")) && assetArchMatches(lower)
	default:
		return strings.Contains(lower, goos)
	}
}

func assetArchMatches(lowerName string) bool {
	if strings.Contains(lowerName, "universal") && runtime.GOOS == "darwin" {
		return true
	}
	switch runtime.GOARCH {
	case "amd64":
		return strings.Contains(lowerName, "amd64") || strings.Contains(lowerName, "x86_64") || strings.Contains(lowerName, "x64")
	case "arm64":
		return strings.Contains(lowerName, "arm64") || strings.Contains(lowerName, "aarch64")
	default:
		return strings.Contains(lowerName, runtime.GOARCH)
	}
}

func normalizeVersion(version string) string {
	version = strings.TrimSpace(version)
	version = strings.TrimPrefix(version, "v")
	if version == "" {
		return "0.0.0"
	}
	return version
}

func compareVersions(a, b string) int {
	aParts := versionNumbers(a)
	bParts := versionNumbers(b)
	for i := 0; i < 3; i++ {
		if aParts[i] > bParts[i] {
			return 1
		}
		if aParts[i] < bParts[i] {
			return -1
		}
	}
	return 0
}

func versionNumbers(version string) [3]int {
	version = normalizeVersion(version)
	version = strings.Split(version, "-")[0]
	parts := strings.Split(version, ".")
	var out [3]int
	for i := 0; i < len(parts) && i < 3; i++ {
		for _, ch := range parts[i] {
			if ch < '0' || ch > '9' {
				break
			}
			out[i] = out[i]*10 + int(ch-'0')
		}
	}
	return out
}
