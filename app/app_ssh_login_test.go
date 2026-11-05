package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"encoding/pem"
	"net"
	"os"
	"strconv"
	"sync/atomic"
	"testing"

	mimirssh "mimir/ssh"
	"mimir/terminal"

	gossh "golang.org/x/crypto/ssh"
)

// serveLoginCounter runs a loopback SSH server that accepts one key and
// counts successful logins. exec requests answer like a host with tmux;
// pty-req and the shell command are accepted and kept open.
func serveLoginCounter(t *testing.T, authorized gossh.PublicKey) (port int, hostKey gossh.PublicKey, logins *int32) {
	t.Helper()
	_, hostPriv, _ := ed25519.GenerateKey(rand.Reader)
	hostSigner, err := gossh.NewSignerFromKey(hostPriv)
	if err != nil {
		t.Fatal(err)
	}
	var count int32
	cfg := &gossh.ServerConfig{PublicKeyCallback: func(_ gossh.ConnMetadata, key gossh.PublicKey) (*gossh.Permissions, error) {
		if string(key.Marshal()) != string(authorized.Marshal()) {
			return nil, net.ErrClosed
		}
		return nil, nil
	}}
	cfg.AddHostKey(hostSigner)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				conn, chans, reqs, err := gossh.NewServerConn(c, cfg)
				if err != nil {
					c.Close()
					return
				}
				atomic.AddInt32(&count, 1)
				defer conn.Close()
				go gossh.DiscardRequests(reqs)
				for nc := range chans {
					ch, chReqs, err := nc.Accept()
					if err != nil {
						continue
					}
					go func() {
						for req := range chReqs {
							switch req.Type {
							case "exec":
								// Both the tmux probe and the pane's command are exec.
								req.Reply(true, nil)
								ch.Write([]byte("tmux 3.3a\n"))
								status := make([]byte, 4)
								binary.BigEndian.PutUint32(status, 0)
								ch.SendRequest("exit-status", false, status)
								ch.Close()
							default:
								req.Reply(true, nil)
							}
						}
					}()
				}
			}(c)
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port, hostSigner.PublicKey(), &count
}

// One pane, one login: the tmux probe must not log in a second time (a
// hardware key would ask for one more touch).
func TestStartSSHTerminalLogsInOnce(t *testing.T) {
	isolateUserDirs(t)

	_, clientPriv, _ := ed25519.GenerateKey(rand.Reader)
	block, err := gossh.MarshalPrivateKey(clientPriv, "")
	if err != nil {
		t.Fatal(err)
	}
	keyPath := t.TempDir() + "/id_ed25519"
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(block), 0600); err != nil {
		t.Fatal(err)
	}
	clientPub, _ := gossh.NewPublicKey(clientPriv.Public())
	port, hostKey, logins := serveLoginCounter(t, clientPub)

	store, err := mimirssh.NewProfileStore()
	if err != nil {
		t.Fatal(err)
	}
	known, err := mimirssh.NewKnownHostStore()
	if err != nil {
		t.Fatal(err)
	}
	if err := known.AddHostKey("127.0.0.1", port, hostKey); err != nil {
		t.Fatal(err)
	}
	useTmux := true
	profiles, err := store.Create(mimirssh.Profile{
		Name: "counter", Host: "127.0.0.1", Port: port, Username: "root",
		AuthMethod: "key", KeyPath: keyPath, UseTmux: &useTmux,
	})
	if err != nil {
		t.Fatal(err)
	}
	app := &App{TerminalManager: terminal.NewManager(), sshProfileStore: store, knownHostStore: known}

	id, err := app.StartSSHTerminal(profileIDByName(t, profiles, "counter"))
	if err != nil {
		t.Fatalf("StartSSHTerminal: %v", err)
	}
	defer app.TerminalManager.CloseTerminal(id)

	if n := atomic.LoadInt32(logins); n != 1 {
		t.Fatalf("logins = %d, want 1 (port %s)", n, strconv.Itoa(port))
	}
	if meta := app.TerminalManager.GetSSHMeta(id); meta == nil || !meta.Config.TmuxActive || meta.Config.TmuxVersion != "3.3a" {
		t.Fatalf("tmux probe over the pane's connection did not report tmux: %#v", meta)
	}
}
