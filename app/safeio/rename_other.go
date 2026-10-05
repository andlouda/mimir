//go:build !windows

package safeio

import "time"

// POSIX rename over an open file succeeds; a single retry is enough to
// keep one code path.
const (
	renameAttempts = 2
	renameBackoff  = 20 * time.Millisecond
)
