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

// Wayland: WebKitGTK's DMA-BUF renderer (2.42+) is the usual source of
// artifacts, stale regions and tearing on Wayland sessions, especially on
// NVIDIA and in VMs. Setting WEBKIT_DISABLE_DMABUF_RENDERER=1 before GTK
// initialises switches WebKit to its older, slower but reliable path.
// "auto" does that only when the session is Wayland; the user's own
// environment variable always wins.

const (
	waylandFixFileName = "wayland_fix"
	WaylandFixAuto     = "auto"
	WaylandFixOn       = "on"
	WaylandFixOff      = "off"
)

func normalizeWaylandFix(v string) string {
	switch strings.TrimSpace(strings.ToLower(v)) {
	case WaylandFixOn:
		return WaylandFixOn
	case WaylandFixOff:
		return WaylandFixOff
	default:
		return WaylandFixAuto
	}
}

func waylandFixPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("wayland fix config dir: %w", err)
	}
	return filepath.Join(configDir, "mimir", waylandFixFileName), nil
}

func loadWaylandFix() string {
	path, err := waylandFixPath()
	if err != nil {
		return WaylandFixAuto
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return WaylandFixAuto
	}
	return normalizeWaylandFix(string(data))
}

// GetWaylandFix returns the persisted mode (auto | on | off).
func (a *App) GetWaylandFix() string {
	return loadWaylandFix()
}

// SetWaylandFix persists the mode; it takes effect at the next start.
func (a *App) SetWaylandFix(mode string) (string, error) {
	mode = normalizeWaylandFix(mode)
	path, err := waylandFixPath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return "", fmt.Errorf("wayland fix config dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(mode+"\n"), 0600); err != nil {
		return "", fmt.Errorf("save wayland fix: %w", err)
	}
	return mode, nil
}

// IsWaylandSession reports whether the app runs under a Wayland session.
func (a *App) IsWaylandSession() bool {
	return isWaylandSession()
}

func isWaylandSession() bool {
	return os.Getenv("WAYLAND_DISPLAY") != "" || strings.EqualFold(os.Getenv("XDG_SESSION_TYPE"), "wayland")
}

// shouldDisableDMABUF decides, from the persisted mode and the session,
// whether to set WEBKIT_DISABLE_DMABUF_RENDERER before GTK starts.
func shouldDisableDMABUF(mode string, wayland bool, alreadySet bool) bool {
	if alreadySet {
		return false
	}
	switch mode {
	case WaylandFixOn:
		return true
	case WaylandFixOff:
		return false
	default:
		return wayland
	}
}

// applyWaylandFix is called from main() before GTK initialises.
func applyWaylandFix() {
	_, set := os.LookupEnv("WEBKIT_DISABLE_DMABUF_RENDERER")
	if shouldDisableDMABUF(loadWaylandFix(), isWaylandSession(), set) {
		_ = os.Setenv("WEBKIT_DISABLE_DMABUF_RENDERER", "1")
	}
}
