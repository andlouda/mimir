package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"testing"

	gossh "golang.org/x/crypto/ssh"
)

func TestIsHardwareKeyFile(t *testing.T) {
	// The relevant part of an ed25519-sk private key file: OpenSSH framing
	// with the sk key type in the public section.
	body := append([]byte("openssh-key-v1\x00\x00\x00\x00\x04none\x00\x00\x00\x04none\x00\x00\x00\x00\x00\x00\x00\x01\x00\x00\x00\x1a"), []byte("sk-ssh-ed25519@openssh.com")...)
	sk := pem.EncodeToMemory(&pem.Block{Type: "OPENSSH PRIVATE KEY", Bytes: body})
	if !IsHardwareKeyFile(sk) {
		t.Fatal("sk key file not recognised")
	}

	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	block, err := gossh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatal(err)
	}
	if IsHardwareKeyFile(pem.EncodeToMemory(block)) {
		t.Fatal("a plain ed25519 key is not a hardware key")
	}
	if IsHardwareKeyFile([]byte("not a key")) {
		t.Fatal("garbage is not a hardware key")
	}
}
