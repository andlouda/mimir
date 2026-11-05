//go:build windows

package ssh

import (
	"io"
	"os"
	"strings"
)

// The Windows OpenSSH agent (service "ssh-agent") listens on this pipe.
// SSH_AUTH_SOCK may name another pipe (gpg-agent, a Pageant bridge).
const windowsAgentPipe = `\\.\pipe\openssh-ssh-agent`

const agentHint = `start the Windows service "OpenSSH Authentication Agent" (ssh-agent) and add your key with ssh-add`

func dialPlatformAgent() (io.ReadWriteCloser, error) {
	pipe := windowsAgentPipe
	if sock := os.Getenv("SSH_AUTH_SOCK"); strings.HasPrefix(sock, `\\.\pipe\`) {
		pipe = sock
	}
	return os.OpenFile(pipe, os.O_RDWR, 0)
}
