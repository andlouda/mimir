package update

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	updateMaxBytes    = 200 * 1024 * 1024
	checksumMaxBytes  = 64 * 1024
	downloadTimeout   = 5 * time.Minute
	checksumTimeout   = 15 * time.Second
	progressInterval  = 100 * 1024 // emit progress every 100KB
)

// Progress represents download/install progress sent to the frontend.
type Progress struct {
	Stage      string  `json:"stage"`
	Percent    float64 `json:"percent"`
	BytesDone  int64   `json:"bytesDone"`
	BytesTotal int64   `json:"bytesTotal"`
	Error      string  `json:"error,omitempty"`
}

// DownloadAndStage downloads the update, verifies it, and stages it for apply on restart.
func DownloadAndStage(ctx context.Context, info Info, emit func(Progress)) error {
	if info.PlatformAsset == nil {
		return fmt.Errorf("no platform asset available")
	}

	emit(Progress{Stage: "downloading", Percent: 0})

	// 1. Get expected SHA256 from checksums.txt or asset digest
	expectedSHA, err := resolveExpectedSHA(ctx, info)
	if err != nil {
		return fmt.Errorf("resolve checksum: %w", err)
	}

	// 2. Download archive to temp file
	archivePath, err := downloadArchive(ctx, info.PlatformAsset, expectedSHA, emit)
	if err != nil {
		return err
	}
	defer os.Remove(archivePath)

	// 3. Extract binary to staging dir
	emit(Progress{Stage: "extracting", Percent: -1})
	dir, err := stagingDir()
	if err != nil {
		return err
	}
	pendingDir := filepath.Join(dir, "pending")
	binaryPath, err := extractBinary(archivePath, info.PlatformAsset.Name, pendingDir)
	if err != nil {
		return fmt.Errorf("extract binary: %w", err)
	}

	// 4. Hash the extracted binary for the marker
	emit(Progress{Stage: "verifying", Percent: -1})
	binarySHA, err := fileSHA256(binaryPath)
	if err != nil {
		os.Remove(binaryPath)
		return fmt.Errorf("hash extracted binary: %w", err)
	}

	// 5. Write pending marker
	emit(Progress{Stage: "installing", Percent: -1})
	if err := WritePendingMarker(PendingUpdate{
		Version:    info.LatestVersion,
		SHA256:     binarySHA,
		BinaryPath: binaryPath,
	}); err != nil {
		os.Remove(binaryPath)
		return fmt.Errorf("write pending marker: %w", err)
	}

	emit(Progress{Stage: "done", Percent: 100})
	return nil
}

func resolveExpectedSHA(ctx context.Context, info Info) (string, error) {
	if info.ChecksumAsset != nil && info.ChecksumAsset.URL != "" {
		checksums, err := downloadChecksums(ctx, info.ChecksumAsset.URL)
		if err != nil {
			return "", err
		}
		sha, ok := checksums[info.PlatformAsset.Name]
		if !ok {
			return "", fmt.Errorf("no checksum found for %s in checksums.txt", info.PlatformAsset.Name)
		}
		return sha, nil
	}
	if info.PlatformAsset.SHA256 != "" {
		return info.PlatformAsset.SHA256, nil
	}
	return "", fmt.Errorf("no checksum source available — refusing to download unverified binary")
}

func downloadChecksums(ctx context.Context, url string) (map[string]string, error) {
	ctx, cancel := context.WithTimeout(ctx, checksumTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "mimir-update")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download checksums: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download checksums: HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(&io.LimitedReader{R: resp.Body, N: checksumMaxBytes})
	if err != nil {
		return nil, fmt.Errorf("read checksums: %w", err)
	}

	result := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 {
			continue
		}
		hash := strings.TrimSpace(parts[0])
		name := strings.TrimSpace(parts[1])
		name = strings.TrimPrefix(name, "*")
		if hash != "" && name != "" {
			result[name] = hash
		}
	}
	return result, nil
}

func downloadArchive(ctx context.Context, asset *Asset, expectedSHA string, emit func(Progress)) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, downloadTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.URL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "mimir-update")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("download archive: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download archive: HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > updateMaxBytes {
		return "", fmt.Errorf("archive too large: %d bytes (max %d)", resp.ContentLength, updateMaxBytes)
	}

	tmp, err := os.CreateTemp("", ".mimir-update-*.tmp")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	hasher := sha256.New()
	limited := &io.LimitedReader{R: resp.Body, N: updateMaxBytes + 1}

	totalBytes := resp.ContentLength
	var bytesDone int64
	var lastEmit int64

	buf := make([]byte, 32*1024)
	for {
		n, readErr := limited.Read(buf)
		if n > 0 {
			if _, err := tmp.Write(buf[:n]); err != nil {
				tmp.Close()
				os.Remove(tmpPath)
				return "", fmt.Errorf("write archive: %w", err)
			}
			hasher.Write(buf[:n])
			bytesDone += int64(n)

			if bytesDone-lastEmit >= progressInterval {
				pct := float64(-1)
				if totalBytes > 0 {
					pct = float64(bytesDone) / float64(totalBytes) * 100
				}
				emit(Progress{Stage: "downloading", Percent: pct, BytesDone: bytesDone, BytesTotal: totalBytes})
				lastEmit = bytesDone
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			tmp.Close()
			os.Remove(tmpPath)
			return "", fmt.Errorf("read archive: %w", readErr)
		}
	}

	if bytesDone > updateMaxBytes || limited.N == 0 {
		tmp.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("archive exceeds size limit (%d bytes)", updateMaxBytes)
	}

	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("sync archive: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("close archive: %w", err)
	}

	emit(Progress{Stage: "verifying", Percent: -1})
	actualSHA := hex.EncodeToString(hasher.Sum(nil))
	if actualSHA != expectedSHA {
		os.Remove(tmpPath)
		return "", fmt.Errorf("archive checksum mismatch: got %s, expected %s", actualSHA, expectedSHA)
	}

	return tmpPath, nil
}

func extractBinary(archivePath, archiveName, destDir string) (string, error) {
	lowerName := strings.ToLower(archiveName)

	switch {
	case strings.HasSuffix(lowerName, ".tar.gz") || strings.HasSuffix(lowerName, ".tgz"):
		return extractFromTarGz(archivePath, destDir)
	case strings.HasSuffix(lowerName, ".zip"):
		return extractFromZip(archivePath, destDir)
	default:
		destPath := filepath.Join(destDir, PendingBinaryName())
		if err := copyFileSimple(archivePath, destPath); err != nil {
			return "", err
		}
		return destPath, nil
	}
}

func extractFromTarGz(archivePath, destDir string) (string, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", fmt.Errorf("open gzip: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	target := PendingBinaryName()

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("read tar: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		base := filepath.Base(hdr.Name)
		if base != target {
			continue
		}

		destPath := filepath.Join(destDir, target)
		out, err := os.Create(destPath)
		if err != nil {
			return "", err
		}
		if _, err := io.Copy(out, &io.LimitedReader{R: tr, N: updateMaxBytes}); err != nil {
			out.Close()
			os.Remove(destPath)
			return "", err
		}
		if err := out.Close(); err != nil {
			os.Remove(destPath)
			return "", err
		}
		return destPath, nil
	}

	return "", fmt.Errorf("binary %q not found in tar.gz archive", target)
}

func extractFromZip(archivePath, destDir string) (string, error) {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", fmt.Errorf("open zip: %w", err)
	}
	defer r.Close()

	target := PendingBinaryName()

	for _, zf := range r.File {
		base := filepath.Base(zf.Name)
		if base != target {
			continue
		}
		if zf.FileInfo().IsDir() {
			continue
		}

		rc, err := zf.Open()
		if err != nil {
			return "", err
		}

		destPath := filepath.Join(destDir, target)
		out, err := os.Create(destPath)
		if err != nil {
			rc.Close()
			return "", err
		}
		if _, err := io.Copy(out, &io.LimitedReader{R: rc, N: updateMaxBytes}); err != nil {
			out.Close()
			rc.Close()
			os.Remove(destPath)
			return "", err
		}
		rc.Close()
		if err := out.Close(); err != nil {
			os.Remove(destPath)
			return "", err
		}
		return destPath, nil
	}

	return "", fmt.Errorf("binary %q not found in zip archive", target)
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func copyFileSimple(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	return out.Close()
}
