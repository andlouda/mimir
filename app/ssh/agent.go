package ssh

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"

	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// SSH agent authentication.
//
// The private key stays where the agent keeps it: in the Windows OpenSSH
// agent, ssh-agent / gpg-agent on Linux and macOS, or on a hardware key
// (YubiKey, FIDO2) that the agent drives. Mimir only asks the agent for
// its public keys and for one signature per login, which is how an
// ed25519-sk key works at all: its key file is just a handle to the
// authenticator, nothing Mimir could sign with itself. Through a jump
// host the login to the target is end-to-end from this machine, so the
// agent is never exposed to the jump host (no agent forwarding).

// dialAgent is the platform connection to the agent (agent_unix.go,
// agent_windows.go); a variable so tests can substitute it.
var dialAgent = dialPlatformAgent

// ErrAgentUnavailable is returned when no agent answers.
var ErrAgentUnavailable = errors.New("no SSH agent reachable")

// AgentAuthMethod authenticates with the keys of the running SSH agent.
// The agent is contacted when the server asks for a key, and again for
// every signature, so no connection to it is held open.
func AgentAuthMethod() gossh.AuthMethod {
	return gossh.PublicKeysCallback(agentSigners)
}

func agentSigners() ([]gossh.Signer, error) {
	conn, err := dialAgent()
	if err != nil {
		return nil, agentDialError(err)
	}
	defer conn.Close()
	keys, err := agent.NewClient(conn).List()
	if err != nil {
		return nil, fmt.Errorf("SSH agent: listing keys failed: %w", err)
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("SSH agent holds no keys: add yours with ssh-add (for a YubiKey: ssh-add with the id_ed25519_sk file)")
	}
	signers := make([]gossh.Signer, 0, len(keys))
	for _, k := range keys {
		signers = append(signers, &agentSigner{key: k})
	}
	return signers, nil
}

func agentDialError(err error) error {
	if errors.Is(err, fs.ErrNotExist) || strings.Contains(strings.ToLower(err.Error()), "no such file") {
		return fmt.Errorf("%w: %s", ErrAgentUnavailable, agentHint)
	}
	return fmt.Errorf("%w: %v (%s)", ErrAgentUnavailable, err, agentHint)
}

// agentSigner signs through a fresh agent connection per signature. A
// hardware key asks for its touch at this point.
type agentSigner struct {
	key *agent.Key
}

func (s *agentSigner) PublicKey() gossh.PublicKey { return s.key }

func (s *agentSigner) Sign(_ io.Reader, data []byte) (*gossh.Signature, error) {
	return s.sign(data, 0)
}

// SignWithAlgorithm lets the client ask for rsa-sha2-256/512 with an RSA
// key in the agent; other key types ignore the algorithm.
func (s *agentSigner) SignWithAlgorithm(_ io.Reader, data []byte, algorithm string) (*gossh.Signature, error) {
	var flags agent.SignatureFlags
	switch algorithm {
	case gossh.KeyAlgoRSASHA256:
		flags = agent.SignatureFlagRsaSha256
	case gossh.KeyAlgoRSASHA512:
		flags = agent.SignatureFlagRsaSha512
	}
	return s.sign(data, flags)
}

func (s *agentSigner) sign(data []byte, flags agent.SignatureFlags) (*gossh.Signature, error) {
	conn, err := dialAgent()
	if err != nil {
		return nil, agentDialError(err)
	}
	defer conn.Close()
	sig, err := agent.NewClient(conn).SignWithFlags(s.key, data, flags)
	if err != nil {
		return nil, fmt.Errorf("SSH agent refused to sign with %s (hardware key not touched or not plugged in?): %w", s.key.Comment, err)
	}
	return sig, nil
}
