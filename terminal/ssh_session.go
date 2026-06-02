package terminal

import (
	"fmt"
	"io"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// SSHMeta stores the profile ID and connection config needed for reconnecting.
type SSHMeta struct {
	ProfileID string
	Config    SSHConnectConfig
}

// SSHConnectConfig holds the parameters needed to establish an SSH session.
type SSHConnectConfig struct {
	Host            string
	Port            int
	Username        string
	Rows, Cols      uint16
	AuthMethods     []ssh.AuthMethod
	HostKeyCallback ssh.HostKeyCallback
	ConnectTimeout  time.Duration
	Command         string
	TmuxSessionName string
	TmuxActive      bool
	TmuxMode        string
	TmuxStatus      string
	TmuxError       string
	RCMode          string
	RCStatus        string
}

// SSHSession implements TerminalSession over an SSH connection.
type SSHSession struct {
	client        *ssh.Client
	session       *ssh.Session
	stdin         io.WriteCloser
	stdout        io.Reader
	mu            sync.Mutex
	closed        bool
	keepaliveDone chan struct{}
}

// NewSSHSession dials the remote host, authenticates, requests a PTY and starts a shell.
func NewSSHSession(cfg SSHConnectConfig) (*SSHSession, error) {
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)

	hostKeyCallback := cfg.HostKeyCallback
	if hostKeyCallback == nil {
		return nil, fmt.Errorf("ssh connection refused: no host key callback configured")
	}

	clientCfg := &ssh.ClientConfig{
		User:            cfg.Username,
		Auth:            cfg.AuthMethods,
		HostKeyCallback: hostKeyCallback,
		Timeout:         cfg.ConnectTimeout,
	}

	client, err := ssh.Dial("tcp", addr, clientCfg)
	if err != nil {
		return nil, fmt.Errorf("ssh dial failed: %w", err)
	}

	session, err := client.NewSession()
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("ssh new session failed: %w", err)
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		session.Close()
		client.Close()
		return nil, fmt.Errorf("ssh stdin pipe failed: %w", err)
	}

	stdout, err := session.StdoutPipe()
	if err != nil {
		stdin.Close()
		session.Close()
		client.Close()
		return nil, fmt.Errorf("ssh stdout pipe failed: %w", err)
	}

	// Merge stderr into stdout so all output appears in the terminal.
	session.Stderr = session.Stdout

	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}

	if err := session.RequestPty("xterm-256color", int(cfg.Rows), int(cfg.Cols), modes); err != nil {
		stdin.Close()
		session.Close()
		client.Close()
		return nil, fmt.Errorf("ssh request pty failed: %w", err)
	}

	if cfg.Command != "" {
		err = session.Start(cfg.Command)
	} else {
		err = session.Shell()
	}
	if err != nil {
		stdin.Close()
		session.Close()
		client.Close()
		return nil, fmt.Errorf("ssh start failed: %w", err)
	}

	s := &SSHSession{
		client:        client,
		session:       session,
		stdin:         stdin,
		stdout:        stdout,
		keepaliveDone: make(chan struct{}),
	}
	go s.keepalive()
	return s, nil
}

// keepalive sends periodic requests to detect dead connections quickly.
func (s *SSHSession) keepalive() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.keepaliveDone:
			return
		case <-ticker.C:
			_, _, err := s.client.SendRequest("keepalive@openssh.com", true, nil)
			if err != nil {
				s.Close()
				return
			}
		}
	}
}

// Client returns the underlying SSH client.
func (s *SSHSession) Client() *ssh.Client { return s.client }

func (s *SSHSession) Read(p []byte) (int, error) {
	return s.stdout.Read(p)
}

func (s *SSHSession) Write(p []byte) (int, error) {
	return s.stdin.Write(p)
}

func (s *SSHSession) Resize(rows, cols uint16) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	return s.session.WindowChange(int(rows), int(cols))
}

func (s *SSHSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true

	close(s.keepaliveDone)
	_ = s.stdin.Close()
	_ = s.session.Close()
	return s.client.Close()
}
