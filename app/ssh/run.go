package ssh

import (
	"fmt"
	"time"

	gossh "golang.org/x/crypto/ssh"
)

// RunCommand executes a command in a fresh exec session on the
// existing SSH connection. No new login or credentials are involved.
func RunCommand(client *gossh.Client, command string, timeout time.Duration) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("ssh session failed: %w", err)
	}
	defer session.Close()

	type result struct {
		output []byte
		err    error
	}
	done := make(chan result, 1)
	go func() {
		output, err := session.CombinedOutput(command)
		done <- result{output, err}
	}()

	select {
	case r := <-done:
		return string(r.output), r.err
	case <-time.After(timeout):
		// Close unblocks the CombinedOutput goroutine.
		session.Close()
		return "", fmt.Errorf("remote command timed out after %s", timeout)
	}
}
