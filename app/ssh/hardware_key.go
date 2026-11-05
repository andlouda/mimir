package ssh

import (
	"bytes"
	"encoding/pem"
)

// Key types whose private-key file is only a handle to an authenticator
// (YubiKey, other FIDO2 keys); signing needs the hardware, so these keys
// work through the SSH agent, not as a key file.
var hardwareKeyTypes = [][]byte{
	[]byte("sk-ssh-ed25519@openssh.com"),
	[]byte("sk-ecdsa-sha2-nistp256@openssh.com"),
}

// IsHardwareKeyFile reports whether data is an OpenSSH private key of a
// FIDO/sk type. The key type sits in the unencrypted public part of the
// file, so this also works for a passphrase-protected key.
func IsHardwareKeyFile(data []byte) bool {
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "OPENSSH PRIVATE KEY" {
		return false
	}
	for _, t := range hardwareKeyTypes {
		if bytes.Contains(block.Bytes, t) {
			return true
		}
	}
	return false
}
