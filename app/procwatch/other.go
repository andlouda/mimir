//go:build !linux && !windows

package procwatch

// New reports that this platform has no sampler yet (Windows follows with
// a native Toolhelp/iphlpapi implementation; macOS has no cheap socket
// source without lsof).
func New() (Sampler, error) {
	return nil, ErrUnsupported
}
