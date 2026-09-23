package agents

import (
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"time"
)

// FileInfo is the subset of file metadata the transcript lookup needs.
type FileInfo struct {
	Name    string
	IsDir   bool
	ModTime time.Time
	Size    int64
}

// FS abstracts the file access needed to find and read transcripts, so the
// same lookup works for the local disk, a WSL distro (UNC path) and SFTP.
type FS interface {
	Join(elem ...string) string
	ReadDir(dir string) ([]FileInfo, error)
	// Stat returns size and modification time of a file (used by the
	// state watcher to notice changes cheaply).
	Stat(file string) (FileInfo, error)
	// ReadHead returns at most max bytes from the start of the file.
	ReadHead(file string, max int64) ([]byte, error)
	// ReadTail returns at most max bytes from the end of the file. Callers
	// must expect the first line to be partial when the file was larger.
	ReadTail(file string, max int64) ([]byte, error)
}

// ErrNotFound is returned when no transcript matches the working directory.
var ErrNotFound = errors.New("no transcript found for this directory")

// LocalFS reads from the local filesystem. Base, when set, is prepended to
// every path (used for WSL distros reached through \\wsl$\<distro>).
type LocalFS struct {
	Base string
}

func (l LocalFS) resolve(p string) string {
	if l.Base == "" {
		return p
	}
	return filepath.Join(l.Base, filepath.FromSlash(p))
}

func (l LocalFS) Join(elem ...string) string {
	if l.Base != "" {
		return path.Join(elem...)
	}
	return filepath.Join(elem...)
}

func (l LocalFS) ReadDir(dir string) ([]FileInfo, error) {
	entries, err := os.ReadDir(l.resolve(dir))
	if err != nil {
		return nil, err
	}
	out := make([]FileInfo, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, FileInfo{Name: e.Name(), IsDir: e.IsDir(), ModTime: info.ModTime(), Size: info.Size()})
	}
	return out, nil
}

func (l LocalFS) Stat(file string) (FileInfo, error) {
	info, err := os.Stat(l.resolve(file))
	if err != nil {
		return FileInfo{}, err
	}
	return FileInfo{Name: info.Name(), IsDir: info.IsDir(), ModTime: info.ModTime(), Size: info.Size()}, nil
}

func (l LocalFS) ReadHead(file string, max int64) ([]byte, error) {
	f, err := os.Open(l.resolve(file))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, max))
}

func (l LocalFS) ReadTail(file string, max int64) ([]byte, error) {
	f, err := os.Open(l.resolve(file))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ReadTailFrom(f, max)
}

// ReadTailFrom reads the last max bytes of a seekable reader.
func ReadTailFrom(r io.ReadSeeker, max int64) ([]byte, error) {
	size, err := r.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, err
	}
	start := int64(0)
	if size > max {
		start = size - max
	}
	if _, err := r.Seek(start, io.SeekStart); err != nil {
		return nil, err
	}
	return io.ReadAll(io.LimitReader(r, max))
}

// newestFirst sorts files by modification time, newest first.
func newestFirst(files []FileInfo) {
	sort.SliceStable(files, func(i, j int) bool {
		return files[i].ModTime.After(files[j].ModTime)
	})
}
