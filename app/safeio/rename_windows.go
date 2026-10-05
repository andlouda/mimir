//go:build windows

package safeio

import "time"

const (
	renameAttempts = 8
	renameBackoff  = 50 * time.Millisecond
)
