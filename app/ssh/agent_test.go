package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"io"
	"net"
	"strings"
	"testing"

	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// withAgent serves an in-memory keyring as the SSH agent for one test.
func withAgent(t *testing.T, keyring agent.Agent) {
	t.Helper()
	old := dialAgent
	t.Cleanup(func() { dialAgent = old })
	dialAgent = func() (io.ReadWriteCloser, error) {
		client, server := net.Pipe()
		go func() {
			defer server.Close()
			_ = agent.ServeAgent(keyring, server)
		}()
		return client, nil
	}
}

func TestAgentSignersSignAndVerify(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	keyring := agent.NewKeyring()
	if err := keyring.Add(agent.AddedKey{PrivateKey: priv, Comment: "test@mimir"}); err != nil {
		t.Fatal(err)
	}
	withAgent(t, keyring)

	signers, err := agentSigners()
	if err != nil {
		t.Fatal(err)
	}
	if len(signers) != 1 {
		t.Fatalf("got %d signers", len(signers))
	}
	data := []byte("session id and auth request")
	sig, err := signers[0].Sign(rand.Reader, data)
	if err != nil {
		t.Fatal(err)
	}
	if err := signers[0].PublicKey().Verify(data, sig); err != nil {
		t.Fatalf("signature from the agent does not verify: %v", err)
	}
}

func TestAgentAuthLogsIntoAServer(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	keyring := agent.NewKeyring()
	_ = keyring.Add(agent.AddedKey{PrivateKey: priv})
	withAgent(t, keyring)
	authorized, _ := gossh.NewPublicKey(priv.Public())

	_, hostPriv, _ := ed25519.GenerateKey(rand.Reader)
	hostSigner, _ := gossh.NewSignerFromKey(hostPriv)
	serverCfg := &gossh.ServerConfig{PublicKeyCallback: func(_ gossh.ConnMetadata, key gossh.PublicKey) (*gossh.Permissions, error) {
		if string(key.Marshal()) == string(authorized.Marshal()) {
			return nil, nil
		}
		return nil, errors.New("unknown key")
	}}
	serverCfg.AddHostKey(hostSigner)

	// A real loopback connection: both SSH sides write their version line
	// first, which deadlocks on an unbuffered net.Pipe.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	done := make(chan error, 1)
	go func() {
		s, err := ln.Accept()
		if err != nil {
			done <- err
			return
		}
		defer s.Close()
		_, _, _, err = gossh.NewServerConn(s, serverCfg)
		done <- err
	}()
	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	clientCfg := &gossh.ClientConfig{User: "jump", Auth: []gossh.AuthMethod{AgentAuthMethod()}, HostKeyCallback: gossh.InsecureIgnoreHostKey()}
	conn, _, _, err := gossh.NewClientConn(c, ln.Addr().String(), clientCfg)
	if err != nil {
		t.Fatalf("login with the agent key failed: %v", err)
	}
	conn.Close()
	if err := <-done; err != nil {
		t.Fatalf("server side: %v", err)
	}
}

func TestAgentWithoutKeysExplains(t *testing.T) {
	withAgent(t, agent.NewKeyring())
	if _, err := agentSigners(); err == nil || !strings.Contains(err.Error(), "ssh-add") {
		t.Fatalf("want an ssh-add hint, got %v", err)
	}
}

func TestAgentUnreachableExplains(t *testing.T) {
	old := dialAgent
	t.Cleanup(func() { dialAgent = old })
	dialAgent = func() (io.ReadWriteCloser, error) { return nil, errors.New("connection refused") }
	_, err := agentSigners()
	if !errors.Is(err, ErrAgentUnavailable) || !strings.Contains(err.Error(), agentHint) {
		t.Fatalf("want ErrAgentUnavailable with the hint, got %v", err)
	}
}
