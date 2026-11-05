//go:build !windows

package ssh

import (
	"errors"
	"io"
	"net"
	"os"
)

const agentHint = "start ssh-agent and make sure SSH_AUTH_SOCK is set in the environment Mimir runs in"

func dialPlatformAgent() (io.ReadWriteCloser, error) {
	sock := os.Getenv("SSH_AUTH_SOCK")
	if sock == "" {
		return nil, errors.New("SSH_AUTH_SOCK is not set")
	}
	return net.Dial("unix", sock)
}
