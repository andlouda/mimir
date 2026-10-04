package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// GPU policy of the Linux webview (WebKitGTK). "never" is Wails' default
// and the safest on drivers that blank or flicker with accelerated
// compositing; "ondemand" lets the webview use the GPU when a page needs
// it, which is what gives xterm a WebGL context. Read once at start-up, so
// a change needs a restart. Ignored off Linux.

const (
	gpuPolicyFileName = "gpu_policy"
	GPUPolicyNever    = "never"
	GPUPolicyOnDemand = "ondemand"
	GPUPolicyAlways   = "always"
)

func normalizeGPUPolicy(p string) string {
	switch strings.TrimSpace(strings.ToLower(p)) {
	case GPUPolicyOnDemand:
		return GPUPolicyOnDemand
	case GPUPolicyAlways:
		return GPUPolicyAlways
	default:
		return GPUPolicyNever
	}
}

func gpuPolicyPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("gpu policy config dir: %w", err)
	}
	return filepath.Join(configDir, "mimir", gpuPolicyFileName), nil
}

// loadGPUPolicy reads the persisted policy; missing or unreadable files
// yield "never".
func loadGPUPolicy() string {
	path, err := gpuPolicyPath()
	if err != nil {
		return GPUPolicyNever
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return GPUPolicyNever
	}
	return normalizeGPUPolicy(string(data))
}

// GetGPUPolicy returns the persisted policy (never | ondemand | always).
func (a *App) GetGPUPolicy() string {
	return loadGPUPolicy()
}

// SetGPUPolicy persists the policy; it takes effect at the next start.
func (a *App) SetGPUPolicy(policy string) (string, error) {
	policy = normalizeGPUPolicy(policy)
	path, err := gpuPolicyPath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return "", fmt.Errorf("gpu policy config dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(policy+"\n"), 0600); err != nil {
		return "", fmt.Errorf("save gpu policy: %w", err)
	}
	return policy, nil
}
