package main

import (
	"encoding/base64"
	"fmt"

	"mimir/ssh"
)

// SecretStoreState reports the credential-store lifecycle state so the
// frontend can decide whether to show a setup/unlock prompt. Returns one of
// "keyring", "needs_setup", "locked", "unlocked", or "unavailable".
func (a *App) SecretStoreState() string {
	if a.sshSecretStore == nil {
		return "unavailable"
	}
	return a.sshSecretStore.State()
}

// SetupSecretMasterPassword initializes the encrypted-file backend with a
// master password (migrating any legacy secrets) and unlocks it.
func (a *App) SetupSecretMasterPassword(masterPassword string) error {
	if a.sshSecretStore == nil {
		return fmt.Errorf("secret store not initialized")
	}
	if err := a.sshSecretStore.SetupMasterPassword(masterPassword); err != nil {
		return err
	}
	a.reloadAIKeyAfterUnlock()
	return nil
}

// UnlockSecrets unlocks the encrypted-file backend with the master password.
func (a *App) UnlockSecrets(masterPassword string) error {
	if a.sshSecretStore == nil {
		return fmt.Errorf("secret store not initialized")
	}
	if err := a.sshSecretStore.Unlock(masterPassword); err != nil {
		return err
	}
	a.reloadAIKeyAfterUnlock()
	return nil
}

// LockSecrets clears the in-memory data key, requiring a new unlock.
func (a *App) LockSecrets() {
	if a.sshSecretStore != nil {
		a.sshSecretStore.Lock()
	}
}

// ChangeSecretMasterPassword re-wraps the data key under a new master password.
func (a *App) ChangeSecretMasterPassword(oldPassword, newPassword string) error {
	if a.sshSecretStore == nil {
		return fmt.Errorf("secret store not initialized")
	}
	return a.sshSecretStore.ChangeMasterPassword(oldPassword, newPassword)
}

// ListFIDOChallenges returns the enrolled FIDO2 slots so the frontend can run
// an authenticator assertion (WebAuthn get / hmac-secret) for unlocking.
func (a *App) ListFIDOChallenges() ([]ssh.FIDOChallenge, error) {
	if a.sshSecretStore == nil {
		return nil, fmt.Errorf("secret store not initialized")
	}
	return a.sshSecretStore.FIDOChallenges()
}

// UnlockSecretsFIDO unlocks using the base64-encoded secret produced by a
// FIDO2 authenticator's hmac-secret/PRF extension.
func (a *App) UnlockSecretsFIDO(hmacSecretB64 string) error {
	if a.sshSecretStore == nil {
		return fmt.Errorf("secret store not initialized")
	}
	secret, err := base64.StdEncoding.DecodeString(hmacSecretB64)
	if err != nil {
		return fmt.Errorf("invalid FIDO secret encoding: %w", err)
	}
	if err := a.sshSecretStore.UnlockFIDO(secret); err != nil {
		return err
	}
	a.reloadAIKeyAfterUnlock()
	return nil
}

// EnrollFIDO adds a FIDO2 authenticator as an additional unlock method. The
// store must already be unlocked. All binary inputs are base64-encoded.
func (a *App) EnrollFIDO(credentialIDB64, rpID, challengeSaltB64, hmacSecretB64, label string) error {
	if a.sshSecretStore == nil {
		return fmt.Errorf("secret store not initialized")
	}
	credID, err := base64.StdEncoding.DecodeString(credentialIDB64)
	if err != nil {
		return fmt.Errorf("invalid credential id encoding: %w", err)
	}
	salt, err := base64.StdEncoding.DecodeString(challengeSaltB64)
	if err != nil {
		return fmt.Errorf("invalid challenge salt encoding: %w", err)
	}
	secret, err := base64.StdEncoding.DecodeString(hmacSecretB64)
	if err != nil {
		return fmt.Errorf("invalid FIDO secret encoding: %w", err)
	}
	return a.sshSecretStore.AddFIDOSlot(credID, rpID, salt, secret, label)
}

// reloadAIKeyAfterUnlock refreshes cached AI settings so the stored OpenAI key
// becomes available once the store is unlocked.
func (a *App) reloadAIKeyAfterUnlock() {
	settings, err := LoadAISettings(a.sshSecretStore)
	if err != nil {
		return
	}
	a.aiMu.Lock()
	a.aiSettings = settings
	a.aiMu.Unlock()
}
