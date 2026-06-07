package terminal

import "io"

// TerminalSession is the common interface for all terminal backends
// (local PTY, Windows ConPTY, SSH).
type TerminalSession interface {
	io.Reader
	io.Writer
	Resize(rows, cols uint16) error
	Close() error
}
