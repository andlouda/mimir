# Security Follow-Up Notes

## File-Based Credential Encryption

The encrypted file fallback must not derive its encryption key only from non-secret machine metadata.
Future hardening should combine a user-provided master password with machine-bound entropy where available.
