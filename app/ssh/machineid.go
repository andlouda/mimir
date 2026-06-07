package ssh

import "strings"

// machineSecret returns a stable, machine-scoped secret used as an additional
// key-derivation input for the encrypted-file secret backend.
//
// It is intentionally a *secondary* layer: the master password remains the
// primary protection. The machine secret binds the encrypted file to the
// device it was created on, so that a copied/synced/backed-up secrets file is
// useless without also having the original machine identifier.
//
// The value is sourced from OS-level identifiers that are local-only and not
// derivable from public information (unlike hostname/home directory). When no
// identifier is available on a platform, this returns nil and key derivation
// gracefully degrades to password-only.
func machineSecret() []byte {
	id := strings.TrimSpace(platformMachineID())
	if id == "" {
		return nil
	}
	return []byte("mimir-machine-v2:" + id)
}
