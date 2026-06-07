package terminal

import (
	"fmt"
	"io"
	"net"
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
	ProxyClient     *ssh.Client
}

// SSHSession implements TerminalSession over an SSH connection.
type SSHSession struct {
	client        *ssh.Client
	proxyClient   *ssh.Client
	session       *ssh.Session
	stdin         io.WriteCloser
	stdout        io.Reader
	mu            sync.Mutex
	closed        bool
	keepaliveDone chan struct{}
}

// enableTCPKeepalive sets OS-level TCP keepalive on the connection so the
// kernel detects dead peers faster than the application-level keepalive alone.
func enableTCPKeepalive(conn net.Conn) {
	if tc, ok := conn.(*net.TCPConn); ok {
		_ = tc.SetKeepAlive(true)
		_ = tc.SetKeepAlivePeriod(5 * time.Second)
	}
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

	var client *ssh.Client
	if cfg.ProxyClient != nil {
		conn, err := cfg.ProxyClient.Dial("tcp", addr)
		if err != nil {
			return nil, fmt.Errorf("ssh proxy dial failed: %w", err)
		}
		enableTCPKeepalive(conn)
		clientConn, chans, reqs, err := ssh.NewClientConn(conn, addr, clientCfg)
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("ssh proxy client handshake failed: %w", err)
		}
		client = ssh.NewClient(clientConn, chans, reqs)
	} else {
		timeout := cfg.ConnectTimeout
		if timeout == 0 {
			timeout = 10 * time.Second
		}
		conn, err := net.DialTimeout("tcp", addr, timeout)
		if err != nil {
			return nil, fmt.Errorf("ssh dial failed: %w", err)
		}
		enableTCPKeepalive(conn)
		clientConn, chans, reqs, err := ssh.NewClientConn(conn, addr, clientCfg)
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("ssh handshake failed: %w", err)
		}
		client = ssh.NewClient(clientConn, chans, reqs)
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
		proxyClient:   cfg.ProxyClient,
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
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.keepaliveDone:
			return
		case <-ticker.C:
			done := make(chan error, 1)
			go func() {
				_, _, err := s.client.SendRequest("keepalive@openssh.com", true, nil)
				done <- err
			}()
			select {
			case err := <-done:
				if err != nil {
					s.Close()
					return
				}
			case <-time.After(10 * time.Second):
				s.Close()
				return
			case <-s.keepaliveDone:
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

// Write sends data to the remote shell with a 10s timeout. If the timeout
// fires, Close() is called which closes the underlying net.Conn — this
// unblocks the stuck stdin.Write goroutine. The goroutine is not leaked
// indefinitely; it exits once the connection teardown completes.
func (s *SSHSession) Write(p []byte) (int, error) {
	type writeResult struct {
		n   int
		err error
	}
	done := make(chan writeResult, 1)
	go func() {
		n, err := s.stdin.Write(p)
		done <- writeResult{n, err}
	}()
	select {
	case r := <-done:
		if r.err != nil {
			go s.Close()
		}
		return r.n, r.err
	case <-time.After(10 * time.Second):
		go s.Close()
		return 0, fmt.Errorf("ssh write timed out")
	}
}

// Resize changes the remote PTY size with a 5s timeout. Same goroutine
// lifecycle as Write — Close() unblocks the stuck WindowChange call.
func (s *SSHSession) Resize(rows, cols uint16) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	done := make(chan error, 1)
	go func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.closed {
			done <- nil
			return
		}
		done <- s.session.WindowChange(int(rows), int(cols))
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		go s.Close()
		return fmt.Errorf("ssh resize timed out")
	}
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
	err := s.client.Close()
	if s.proxyClient != nil {
		if proxyErr := s.proxyClient.Close(); err == nil {
			err = proxyErr
		}
	}
	return err
}
