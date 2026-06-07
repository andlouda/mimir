package terminal

// SetRecordInput enables or disables recording of keystrokes (input frames).
//
// Input recording is OFF by default because typed secrets (passwords entered
// at prompts, pasted tokens, SSH key passphrases) appear in input frames with
// no structure that a pattern-based scrubber could redact. Enabling it is an
// explicit, informed opt-in.
func (m *Manager) SetRecordInput(enabled bool) {
	m.ptyMutex.Lock()
	defer m.ptyMutex.Unlock()
	m.recordInput = enabled
}

// RecordInputEnabled reports whether keystroke recording is enabled.
func (m *Manager) RecordInputEnabled() bool {
	m.ptyMutex.Lock()
	defer m.ptyMutex.Unlock()
	return m.recordInput
}
