package aiflow

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"mimir/activitylog"
)

type DiscoveryResolver interface {
	Resolve(discoveryTool string, terminalType string, variables map[string]string) ([]string, error)
}

type CachedDiscoveryResolver struct {
	workdir string
	cache   map[string][]string
}

func NewCachedDiscoveryResolver(workdir string) *CachedDiscoveryResolver {
	return &CachedDiscoveryResolver{
		workdir: workdir,
		cache:   make(map[string][]string),
	}
}

func (r *CachedDiscoveryResolver) Resolve(discoveryTool string, terminalType string, variables map[string]string) ([]string, error) {
	cacheKey := buildDiscoveryCacheKey(discoveryTool, terminalType, variables)
	if cached, ok := r.cache[cacheKey]; ok {
		return append([]string(nil), cached...), nil
	}

	var values []string
	var err error
	if isComposeServicesTool(discoveryTool) {
		values, err = r.resolveComposeServices(terminalType, variables)
	} else {
		values, err = r.resolveSingle(discoveryTool, terminalType, variables)
	}
	if err != nil {
		return nil, err
	}

	r.cache[cacheKey] = values
	logDiscoverySecurityEvent("discovery_resolved", discoveryTool, fmt.Sprintf("%d values", len(values)), variables)
	return append([]string(nil), values...), nil
}

func (r *CachedDiscoveryResolver) resolveSingle(discoveryTool string, terminalType string, variables map[string]string) ([]string, error) {
	name, args, wantsWorkdir, err := discoverySpec(discoveryTool, variables)
	if err != nil {
		logDiscoverySecurityEvent("discovery_denied", discoveryTool, err.Error(), variables)
		return nil, err
	}

	output, err := r.runSpec(terminalType, discoveryCmdSpec{name: name, args: args, wantsWorkdir: wantsWorkdir})
	if err != nil {
		message := discoveryErrorMessage(output, err, r.workdir)
		logDiscoverySecurityEvent("discovery_failed", discoveryTool, message, variables)
		return nil, fmt.Errorf("discovery %s failed: %s", discoveryTool, message)
	}
	return NormalizeDiscoveryOutput(output), nil
}

// resolveComposeServices lists compose services runtime-first:
//  1. services with containers in the current project (docker compose ps),
//  2. services defined in the project's compose file (docker compose config),
//  3. when neither yields anything (e.g. the terminal is not inside a compose
//     project): every compose service currently running on the host, derived
//     from container labels — works without any compose file.
func (r *CachedDiscoveryResolver) resolveComposeServices(terminalType string, variables map[string]string) ([]string, error) {
	merged := make([]string, 0, 8)
	firstErr := ""
	for _, spec := range composeProjectSpecs {
		output, err := r.runSpec(terminalType, spec)
		if err != nil {
			if firstErr == "" {
				firstErr = discoveryErrorMessage(output, err, r.workdir)
			}
			continue
		}
		merged = append(merged, NormalizeDiscoveryOutput(output)...)
	}
	if len(merged) > 0 {
		return NormalizeDiscoveryOutput(strings.Join(merged, "\n")), nil
	}

	output, err := r.runSpec(terminalType, composeRuntimeFallbackSpec)
	if err != nil {
		if firstErr == "" {
			firstErr = discoveryErrorMessage(output, err, "")
		}
		logDiscoverySecurityEvent("discovery_failed", composeServicesTool, firstErr, variables)
		return nil, fmt.Errorf("discovery %s failed: %s", composeServicesTool, firstErr)
	}
	return NormalizeDiscoveryOutput(output), nil
}

func (r *CachedDiscoveryResolver) runSpec(terminalType string, spec discoveryCmdSpec) (string, error) {
	base := spec.name
	// Resolve the executable on the host only for host-side runs; inside WSL
	// the bare name must resolve via the distro's PATH.
	if !isWSLTerminalType(terminalType) {
		base = discoveryExecutable(spec.name)
	}
	command, args, err := wrapDiscoveryCommand(terminalType, spec.wantsWorkdir, r.workdir, base, spec.args...)
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, command, args...)
	// For WSL the workdir travels via `wsl.exe --cd`; setting Dir would point
	// the Windows-side process at a Linux path.
	if spec.wantsWorkdir && !isWSLTerminalType(terminalType) && filepath.IsAbs(r.workdir) {
		cmd.Dir = r.workdir
	}

	output, err := cmd.CombinedOutput()
	return string(output), err
}

func discoveryErrorMessage(output string, err error, workdir string) string {
	message := strings.TrimSpace(output)
	if message == "" {
		message = err.Error()
	}
	// Include where the command ran; "no configuration file found" style
	// errors are meaningless without it.
	if workdir != "" {
		message = fmt.Sprintf("%s (workdir: %s)", message, workdir)
	}
	return message
}

func buildDiscoveryCacheKey(discoveryTool string, terminalType string, variables map[string]string) string {
	keys := make([]string, 0, len(variables))
	for key := range variables {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var builder strings.Builder
	builder.WriteString(strings.TrimSpace(discoveryTool))
	builder.WriteString("|")
	builder.WriteString(strings.TrimSpace(terminalType))
	for _, key := range keys {
		builder.WriteString("|")
		builder.WriteString(key)
		builder.WriteString("=")
		builder.WriteString(strings.TrimSpace(variables[key]))
	}
	return builder.String()
}

func isValidDiscoveryArg(value string) bool {
	return value != "" && !strings.HasPrefix(value, "-") && !strings.ContainsRune(value, 0)
}

const k8sNamesJSONPath = "jsonpath={range .items[*]}{.metadata.name}{\"\\n\"}{end}"

const composeServicesTool = "discovery:list_compose_services"

func isComposeServicesTool(discoveryTool string) bool {
	return strings.TrimSpace(discoveryTool) == composeServicesTool
}

type discoveryCmdSpec struct {
	name         string
	args         []string
	wantsWorkdir bool
}

// Compose services, runtime first: containers of the current project, then
// the services defined in the project's compose file.
var composeProjectSpecs = []discoveryCmdSpec{
	{name: "docker", args: []string{"compose", "ps", "--services"}, wantsWorkdir: true},
	{name: "docker", args: []string{"compose", "config", "--services"}, wantsWorkdir: true},
}

// Last resort when the terminal is not inside a compose project: every compose
// service currently running on the host, derived from container labels.
var composeRuntimeFallbackSpec = discoveryCmdSpec{
	name: "docker",
	args: []string{"ps", "--filter", "label=com.docker.compose.service", "--format", `{{.Label "com.docker.compose.service"}}`},
}

// discoverySpec returns the allowlisted executable name and arguments for a
// discovery tool, plus whether the command depends on the working directory.
// This is the single source of truth for what discovery is allowed to run.
func discoverySpec(discoveryTool string, variables map[string]string) (string, []string, bool, error) {
	switch strings.TrimSpace(discoveryTool) {
	case "discovery:list_k8s_namespaces":
		return "kubectl", []string{"get", "namespaces", "-o", k8sNamesJSONPath}, false, nil
	case "discovery:list_k8s_pods":
		namespace := strings.TrimSpace(variables["Namespace"])
		if !isValidDiscoveryArg(namespace) {
			return "", nil, false, fmt.Errorf("Namespace is required and must not start with a dash")
		}
		return "kubectl", []string{"get", "pods", "-n", namespace, "-o", k8sNamesJSONPath}, false, nil
	case "discovery:list_docker_containers":
		return "docker", []string{"ps", "--format", "{{.Names}}"}, false, nil
	case "discovery:list_compose_services":
		return "docker", []string{"compose", "config", "--services"}, true, nil
	case "discovery:list_k8s_resources":
		namespace := strings.TrimSpace(variables["Namespace"])
		resourceType := strings.TrimSpace(variables["ResourceType"])
		if !isValidDiscoveryArg(namespace) || !isValidDiscoveryArg(resourceType) {
			return "", nil, false, fmt.Errorf("Namespace and ResourceType are required and must not start with a dash")
		}
		return "kubectl", []string{"get", resourceType, "-n", namespace, "-o", k8sNamesJSONPath}, false, nil
	default:
		return "", nil, false, fmt.Errorf("unknown discovery tool %s", discoveryTool)
	}
}

func discoveryCommand(discoveryTool string, terminalType string, workdir string, variables map[string]string) (string, []string, error) {
	name, args, wantsWorkdir, err := discoverySpec(discoveryTool, variables)
	if err != nil {
		return "", nil, err
	}
	if isWSLTerminalType(terminalType) {
		// Keep the bare name: it must resolve inside the WSL distro's PATH.
		// Resolving on the host would yield a Windows path (e.g. Rancher
		// Desktop's docker.exe) that bash inside WSL cannot execute.
		return wrapDiscoveryCommand(terminalType, wantsWorkdir, workdir, name, args...)
	}
	return wrapDiscoveryCommand(terminalType, wantsWorkdir, workdir, discoveryExecutable(name), args...)
}

func discoveryExecutable(name string) string {
	if path, err := exec.LookPath(name); err == nil && path != "" {
		return path
	}
	for _, dir := range []string{"/usr/local/bin", "/usr/bin", "/bin", "/snap/bin", "/opt/homebrew/bin"} {
		path := filepath.Join(dir, name)
		if resolved, err := exec.LookPath(path); err == nil && resolved != "" {
			return resolved
		}
	}
	return name
}

func isWSLTerminalType(terminalType string) bool {
	return strings.TrimSpace(strings.ToLower(terminalType)) == "wsl"
}

func wrapDiscoveryCommand(terminalType string, wantsWorkdir bool, workdir string, baseCommand string, args ...string) (string, []string, error) {
	if isWSLTerminalType(terminalType) {
		wrapped := []string{}
		// The WSL workdir is a Linux path; pass it via --cd instead of cmd.Dir.
		if wantsWorkdir && strings.HasPrefix(workdir, "/") {
			wrapped = append(wrapped, "--cd", workdir)
		}
		wrapped = append(wrapped, "--", baseCommand)
		wrapped = append(wrapped, args...)
		return "wsl.exe", wrapped, nil
	}
	return baseCommand, args, nil
}

// BuildRemoteDiscoveryScript builds a POSIX shell script that runs an
// allowlisted discovery command on a remote host. When the terminal lives in a
// tmux session, the script first resolves the pane's current directory via
// tmux (no shell integration required on the remote) and runs the command
// there. All values are single-quoted; only allowlisted commands can occur.
func BuildRemoteDiscoveryScript(discoveryTool string, variables map[string]string, tmuxSessionName string) (string, error) {
	if isComposeServicesTool(discoveryTool) {
		return remoteTmuxCwdPrefix(tmuxSessionName) + remoteComposeServicesCommand(), nil
	}

	name, args, wantsWorkdir, err := discoverySpec(discoveryTool, variables)
	if err != nil {
		return "", err
	}

	command := quoteSpecCommand(discoveryCmdSpec{name: name, args: args})
	if !wantsWorkdir {
		return command, nil
	}
	return remoteTmuxCwdPrefix(tmuxSessionName) + command, nil
}

// remoteTmuxCwdPrefix resolves the remote terminal's working directory via the
// pane's tmux session (no shell integration needed) and cds into it. Empty
// when the terminal has no tmux session.
func remoteTmuxCwdPrefix(tmuxSessionName string) string {
	session := strings.TrimSpace(tmuxSessionName)
	if session == "" || !isValidDiscoveryArg(session) {
		return ""
	}
	target := quotePosixArg(session + ":")
	return `__mimir_cwd=$(tmux display-message -p -t ` + target + ` '#{pane_current_path}' 2>/dev/null); ` +
		`if [ -n "$__mimir_cwd" ]; then cd "$__mimir_cwd" 2>/dev/null || true; fi; `
}

// remoteComposeServicesCommand mirrors resolveComposeServices for remote
// hosts: project services (running + defined) first, then the label-based
// runtime fallback that works without a compose file.
func remoteComposeServicesCommand() string {
	union := quoteSpecCommand(composeProjectSpecs[0]) + " 2>/dev/null; " + quoteSpecCommand(composeProjectSpecs[1]) + " 2>/dev/null"
	fallback := quoteSpecCommand(composeRuntimeFallbackSpec)
	return `__mimir_out=$({ ` + union + `; }); ` +
		`if [ -z "$__mimir_out" ]; then __mimir_out=$(` + fallback + ` 2>/dev/null); fi; ` +
		`printf '%s\n' "$__mimir_out"`
}

func quoteSpecCommand(spec discoveryCmdSpec) string {
	parts := make([]string, 0, len(spec.args)+1)
	parts = append(parts, quotePosixArg(spec.name))
	for _, arg := range spec.args {
		parts = append(parts, quotePosixArg(arg))
	}
	return strings.Join(parts, " ")
}

func quotePosixArg(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

// NormalizeDiscoveryOutput trims, deduplicates and sorts line-based command
// output into a list of discovery values.
func NormalizeDiscoveryOutput(output string) []string {
	lines := strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n")
	result := make([]string, 0, len(lines))
	seen := make(map[string]struct{}, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if _, ok := seen[line]; ok {
			continue
		}
		seen[line] = struct{}{}
		result = append(result, line)
	}
	sort.Strings(result)
	return result
}

func logDiscoverySecurityEvent(event string, discoveryTool string, reason string, variables map[string]string) {
	_ = activitylog.Append(activitylog.KindSecurityEvents, activitylog.SecurityEventEntry{
		Timestamp: time.Now().Format(time.RFC3339),
		Event:     event,
		Operation: "ai_discovery",
		Reason:    reason,
		Metadata: map[string]string{
			"discoveryTool": discoveryTool,
			"variables":     formatDiscoveryVariables(variables),
		},
	})
}

func formatDiscoveryVariables(variables map[string]string) string {
	if len(variables) == 0 {
		return ""
	}
	keys := make([]string, 0, len(variables))
	for key := range variables {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+strings.TrimSpace(variables[key]))
	}
	return strings.Join(parts, ",")
}
