package timeline

// Settings drive the agent timeline: which sources are collected
// and how long the records are kept. Everything is off until the user
// turns the master switch on; the sub-switches then default to on so one
// click gives the full picture.
type Settings struct {
	// Enabled is the master switch ("Agent-Zeitleiste").
	Enabled bool `json:"enabled"`
	// HookToolEvents installs PreToolUse/PostToolUse/Stop hooks (tool ids,
	// timing, the agent's pid).
	HookToolEvents bool `json:"hookToolEvents"`
	// OTelReceiver listens for the agent's OpenTelemetry export (durations,
	// costs, permission decisions).
	OTelReceiver bool `json:"otelReceiver"`
	// Port of the loopback OTLP/HTTP listener.
	Port int `json:"port"`
	// RequireToken makes the listener demand the shared token header.
	RequireToken bool `json:"requireToken"`
	// WSLBridge (Windows) also listens on the WSL virtual adapter so Claude
	// Code inside WSL2 reaches the receiver; needs the token.
	WSLBridge bool `json:"wslBridge"`
	// Token is the shared secret; it stays in the file and is never sent to
	// the frontend (HasToken says whether one exists).
	Token    string `json:"token,omitempty"`
	HasToken bool   `json:"hasToken,omitempty"`
	// ToolDetails asks the agent to include tool parameters in telemetry
	// (OTEL_LOG_TOOL_DETAILS). Off: hooks already carry the redacted input.
	ToolDetails bool `json:"toolDetails"`
	// TracesBeta enables the agent's span export (subagent nesting).
	TracesBeta bool `json:"tracesBeta"`
	// ExportIntervalMs is how often the agent flushes telemetry.
	ExportIntervalMs int `json:"exportIntervalMs"`
	// Observe samples the pane's processes and connections.
	Observe           bool `json:"observe"`
	ObserveIntervalMs int  `json:"observeIntervalMs"`
	ObserveUDP        bool `json:"observeUdp"`
	// RetentionDays removes session files older than this (0 = keep).
	RetentionDays int `json:"retentionDays"`
	// MaxFileMB rotates a session file above this size.
	MaxFileMB int `json:"maxFileMb"`
}
