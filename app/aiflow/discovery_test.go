package aiflow

import (
	"strings"
	"testing"
)

func TestNormalizeDiscoveryOutput(t *testing.T) {
	values := NormalizeDiscoveryOutput("api-2\r\napi-1\n\napi-2\n")
	if len(values) != 2 {
		t.Fatalf("expected 2 values, got %d", len(values))
	}
	if values[0] != "api-1" || values[1] != "api-2" {
		t.Fatalf("unexpected normalized values: %+v", values)
	}
}

func TestBuildDiscoveryCacheKeyDeterministic(t *testing.T) {
	left := buildDiscoveryCacheKey("discovery:list_k8s_pods", "bash", map[string]string{
		"Pod":       "api",
		"Namespace": "default",
	})
	right := buildDiscoveryCacheKey("discovery:list_k8s_pods", "bash", map[string]string{
		"Namespace": "default",
		"Pod":       "api",
	})
	if left != right {
		t.Fatalf("expected deterministic cache key, got %q vs %q", left, right)
	}
}

func TestDiscoveryCommandValidation(t *testing.T) {
	if _, _, err := discoveryCommand("discovery:list_k8s_pods", "bash", "", map[string]string{}); err == nil {
		t.Fatalf("expected missing namespace to fail")
	}
	if _, _, err := discoveryCommand("discovery:list_k8s_resources", "bash", "", map[string]string{
		"Namespace": "default",
	}); err == nil {
		t.Fatalf("expected missing resource type to fail")
	}
}

func TestDiscoveryCommandWSLPassesWorkdirViaCd(t *testing.T) {
	command, args, err := discoveryCommand("discovery:list_compose_services", "wsl", "/home/user/project", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if command != "wsl.exe" {
		t.Fatalf("expected wsl.exe wrapper, got %q", command)
	}
	joined := strings.Join(args, " ")
	if joined != "--cd /home/user/project -- docker compose config --services" {
		t.Fatalf("unexpected wsl args: %q", joined)
	}
}

func TestDiscoveryCommandWSLUsesBareExecutableName(t *testing.T) {
	// The executable must resolve inside the WSL distro, not on the host —
	// a host-resolved Windows path (e.g. Rancher Desktop's docker.exe) cannot
	// be executed by bash inside WSL.
	_, args, err := discoveryCommand("discovery:list_docker_containers", "wsl", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(args) < 2 || args[0] != "--" || args[1] != "docker" {
		t.Fatalf("expected bare docker name after --, got %v", args)
	}
}

func TestDiscoveryCommandWSLIgnoresNonLinuxWorkdir(t *testing.T) {
	_, args, err := discoveryCommand("discovery:list_compose_services", "wsl", `C:\Users\foo`, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(strings.Join(args, " "), "--cd") {
		t.Fatalf("windows path must not be passed to --cd: %v", args)
	}
}

func TestBuildRemoteDiscoveryScriptWithTmuxCwd(t *testing.T) {
	script, err := BuildRemoteDiscoveryScript("discovery:list_compose_services", nil, "mimir-ssh-prod-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, part := range []string{
		"tmux display-message -p -t 'mimir-ssh-prod-1:'",
		"'#{pane_current_path}'",
		`cd "$__mimir_cwd"`,
		// Runtime first, then defined services, then the label-based fallback.
		"'docker' 'compose' 'ps' '--services'",
		"'docker' 'compose' 'config' '--services'",
		"'docker' 'ps' '--filter' 'label=com.docker.compose.service'",
	} {
		if !strings.Contains(script, part) {
			t.Fatalf("script %q does not contain %q", script, part)
		}
	}
}

func TestBuildRemoteDiscoveryScriptComposeWithoutTmuxStillHasRuntimeFallback(t *testing.T) {
	script, err := BuildRemoteDiscoveryScript("discovery:list_compose_services", nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(script, "tmux") {
		t.Fatalf("script without tmux session must not invoke tmux: %q", script)
	}
	if !strings.Contains(script, "label=com.docker.compose.service") {
		t.Fatalf("expected runtime fallback in script: %q", script)
	}
}

func TestBuildRemoteDiscoveryScriptWithoutTmuxSession(t *testing.T) {
	script, err := BuildRemoteDiscoveryScript("discovery:list_docker_containers", nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(script, "tmux") {
		t.Fatalf("cwd-independent command must not invoke tmux: %q", script)
	}
	if script != "'docker' 'ps' '--format' '{{.Names}}'" {
		t.Fatalf("unexpected script: %q", script)
	}
}

func TestBuildRemoteDiscoveryScriptRejectsUnknownTool(t *testing.T) {
	if _, err := BuildRemoteDiscoveryScript("discovery:rm_rf", nil, ""); err == nil {
		t.Fatalf("expected unknown tool to be rejected")
	}
}

func TestBuildRemoteDiscoveryScriptQuotesVariables(t *testing.T) {
	script, err := BuildRemoteDiscoveryScript("discovery:list_k8s_pods", map[string]string{
		"Namespace": "prod",
	}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(script, "'kubectl' 'get' 'pods' '-n' 'prod'") {
		t.Fatalf("unexpected script: %q", script)
	}
}

func TestQuotePosixArgEscapesSingleQuotes(t *testing.T) {
	quoted := quotePosixArg("a'b")
	if quoted != `'a'\''b'` {
		t.Fatalf("unexpected quoting: %q", quoted)
	}
}
