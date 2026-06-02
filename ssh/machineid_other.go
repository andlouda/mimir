//go:build !windows && !linux && !darwin

package ssh

// platformMachineID has no implementation on unsupported platforms; key
// derivation degrades gracefully to password-only.
func platformMachineID() string {
	return ""
}
