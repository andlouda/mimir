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

	command, args, err := discoveryCommand(discoveryTool, terminalType, variables)
	if err != nil {
		logDiscoverySecurityEvent("discovery_denied", discoveryTool, err.Error(), variables)
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, command, args...)
	if shouldSetDiscoveryWorkdir(discoveryTool, r.workdir) {
		cmd.Dir = r.workdir
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		logDiscoverySecurityEvent("discovery_failed", discoveryTool, message, variables)
		return nil, fmt.Errorf("discovery %s failed: %s", discoveryTool, message)
	}

	values := normalizeDiscoveryOutput(string(output))
	r.cache[cacheKey] = values
	logDiscoverySecurityEvent("discovery_resolved", discoveryTool, fmt.Sprintf("%d values", len(values)), variables)
	return append([]string(nil), values...), nil
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

func discoveryCommand(discoveryTool string, terminalType string, variables map[string]string) (string, []string, error) {
	switch strings.TrimSpace(discoveryTool) {
	case "discovery:list_k8s_namespaces":
		return wrapDiscoveryCommand(terminalType, "kubectl", "get", "namespaces", "-o", "jsonpath={range .items[*]}{.metadata.name}{\"\\n\"}{end}")
	case "discovery:list_k8s_pods":
		namespace := strings.TrimSpace(variables["Namespace"])
		if namespace == "" {
			return "", nil, fmt.Errorf("Namespace is required for discovery:list_k8s_pods")
		}
		return wrapDiscoveryCommand(terminalType, "kubectl", "get", "pods", "-n", namespace, "-o", "jsonpath={range .items[*]}{.metadata.name}{\"\\n\"}{end}")
	case "discovery:list_docker_containers":
		return wrapDiscoveryCommand(terminalType, "docker", "ps", "--format", "{{.Names}}")
	case "discovery:list_compose_services":
		return wrapDiscoveryCommand(terminalType, "docker", "compose", "config", "--services")
	case "discovery:list_k8s_resources":
		namespace := strings.TrimSpace(variables["Namespace"])
		resourceType := strings.TrimSpace(variables["ResourceType"])
		if namespace == "" || resourceType == "" {
			return "", nil, fmt.Errorf("Namespace and ResourceType are required for discovery:list_k8s_resources")
		}
		return wrapDiscoveryCommand(terminalType, "kubectl", "get", resourceType, "-n", namespace, "-o", "jsonpath={range .items[*]}{.metadata.name}{\"\\n\"}{end}")
	default:
		return "", nil, fmt.Errorf("unknown discovery tool %s", discoveryTool)
	}
}

func wrapDiscoveryCommand(terminalType string, baseCommand string, args ...string) (string, []string, error) {
	switch strings.TrimSpace(strings.ToLower(terminalType)) {
	case "wsl":
		wrapped := append([]string{"--", baseCommand}, args...)
		return "wsl.exe", wrapped, nil
	default:
		return baseCommand, args, nil
	}
}

func shouldSetDiscoveryWorkdir(discoveryTool string, workdir string) bool {
	if strings.TrimSpace(workdir) == "" {
		return false
	}
	switch strings.TrimSpace(discoveryTool) {
	case "discovery:list_compose_services":
		return filepath.IsAbs(workdir)
	default:
		return false
	}
}

func normalizeDiscoveryOutput(output string) []string {
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
