package aiflow

import (
	"fmt"
	"regexp"
	"strings"

	"mimir/template"
	"mimir/tools"
)

var immutableBlockedCommandFragments = []string{
	"rm -rf",
	"rm -f",
	"remove-item",
	"del /s /q",
	"del /f",
	" taskkill",
	"pkill ",
	"killall ",
	" kill ",
	"shutdown",
	"reboot",
	"restart-computer",
	"stop-computer",
	"sc delete",
	"sc stop",
	"kubectl delete",
	"kubectl apply",
	"kubectl patch",
	"kubectl replace",
	"kubectl scale",
	"kubectl rollout restart",
	"docker rm",
	"docker stop",
	"docker kill",
	"docker restart",
	"docker system prune",
	"docker compose down",
	"docker compose restart",
	"docker compose rm",
	"docker compose up",
	"systemctl restart",
	"systemctl stop",
	"systemctl disable",
	"service restart",
	"service stop",
	"truncate -s 0",
	"mkfs",
	"format ",
	"apt install",
	"apt remove",
	"apt purge",
	"apt upgrade",
	"dnf install",
	"yum install",
	"brew install",
	"pip install",
	"npm install",
	"netsh advfirewall",
	"ufw ",
	"iptables ",
	"reg add",
	"reg delete",
	"helm upgrade",
	"helm uninstall",
}

var immutableBlockedValueFragments = []string{
	";",
	"&&",
	"||",
	"|",
	">",
	"<",
	"`",
	"$(",
	"${",
	"\n",
	"\r",
	"\"",
	"'",
}

func ImmutablePromptGuardrails() string {
	return strings.Join([]string{
		"System guardrails (non-editable):",
		"1. Use only registered tool IDs from the provided list.",
		"2. Never invent raw shell commands and never use name-based fallback.",
		"3. Only choose read-only, diagnostic tools.",
		"4. Never choose tools that write, delete, install, restart, stop, kill, deploy, patch, scale, prune, or otherwise mutate files, packages, services, containers, clusters, firewall rules, or system state.",
		"5. Never access, print, or exfiltrate secrets, tokens, API keys, SSH keys, environment credentials, or credential stores.",
		"6. If the request requires mutation, secret access, or anything ambiguous, return an empty toolId and explain the refusal.",
	}, "\n")
}

func ApplyImmutableGuardrails(cfg Config) Config {
	cfg.Prompt.RequireStableToolID = true
	cfg.Prompt.AllowTemplateNameFallback = false
	cfg.Approval.RespectStepFlag = true
	cfg.Approval.RequireApprovalForMedium = true
	cfg.Approval.RequireApprovalForHigh = true
	return cfg
}

func TemplateAllowedForAI(tmpl template.Template) bool {
	if strings.TrimSpace(strings.ToLower(tmpl.DangerLevel)) != "low" {
		return false
	}

	for _, command := range tmpl.Commands {
		lower := strings.ToLower(command)
		for _, fragment := range immutableBlockedCommandFragments {
			if strings.Contains(lower, fragment) {
				return false
			}
		}
	}
	return true
}

func ValidateSelectedTool(tool tools.Tool, variables map[string]string) error {
	return ValidateSelectedToolWithDiscovery(tool, variables, "", nil)
}

func ValidateSelectedToolWithDiscovery(tool tools.Tool, variables map[string]string, terminalType string, resolver DiscoveryResolver) error {
	if tool == nil {
		return fmt.Errorf("selected tool is required")
	}
	if tool.Risk() != tools.RiskLow {
		return fmt.Errorf("selected tool %s is not low-risk", tool.ID())
	}

	inspectable, ok := tool.(tools.GuardrailInspectable)
	if !ok {
		return fmt.Errorf("selected tool %s is not guardrail-inspectable", tool.ID())
	}

	for _, command := range inspectable.CommandMap() {
		lower := strings.ToLower(command)
		for _, fragment := range immutableBlockedCommandFragments {
			if strings.Contains(lower, fragment) {
				return fmt.Errorf("selected tool %s contains blocked command fragment %q", tool.ID(), fragment)
			}
		}
	}

	allowedParams := make(map[string]tools.Parameter, len(tool.Parameters()))
	for _, param := range tool.Parameters() {
		allowedParams[param.Name] = param
		if param.Required {
			value := strings.TrimSpace(variables[param.Name])
			if value == "" {
				return fmt.Errorf("required parameter %s is missing", param.Name)
			}
		}
	}

	for name, value := range variables {
		param, ok := allowedParams[name]
		if !ok {
			return fmt.Errorf("parameter %s is not allowed for tool %s", name, tool.ID())
		}
		if err := ValidateParameterValue(param, value, terminalType, variables, resolver); err != nil {
			return err
		}
	}

	return nil
}

func ValidateParameterValue(param tools.Parameter, value string, terminalType string, variables map[string]string, resolver DiscoveryResolver) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}

	if param.Source == tools.ParameterSourceUserOnly {
		return fmt.Errorf("parameter %s requires explicit user input", param.Name)
	}
	if param.Source == tools.ParameterSourceDiscoveryOnly {
		discoveryTool := strings.TrimSpace(param.DiscoveryTool)
		if discoveryTool == "" {
			return fmt.Errorf("parameter %s requires discovery data before execution", param.Name)
		}
		if resolver == nil {
			return fmt.Errorf("parameter %s requires discovery from %s before execution", param.Name, discoveryTool)
		}
		values, err := resolver.Resolve(discoveryTool, terminalType, variables)
		if err != nil {
			return err
		}
		if !containsExact(values, trimmed) {
			return fmt.Errorf("parameter %s is not present in discovery results from %s", param.Name, discoveryTool)
		}
	}
	if param.MaxLength > 0 && len(trimmed) > param.MaxLength {
		return fmt.Errorf("parameter %s exceeds max length %d", param.Name, param.MaxLength)
	}
	if len(param.Options) > 0 && !containsFold(param.Options, trimmed) {
		return fmt.Errorf("parameter %s must be one of the allowed options", param.Name)
	}
	if strings.TrimSpace(param.Pattern) != "" {
		matched, err := regexp.MatchString(param.Pattern, trimmed)
		if err != nil {
			return fmt.Errorf("parameter %s has invalid validation pattern", param.Name)
		}
		if !matched {
			return fmt.Errorf("parameter %s does not match the required pattern", param.Name)
		}
	}

	for _, fragment := range immutableBlockedValueFragments {
		if strings.Contains(trimmed, fragment) {
			return fmt.Errorf("parameter %s contains blocked fragment %q", param.Name, fragment)
		}
	}

	return nil
}

func containsExact(values []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, value := range values {
		if strings.TrimSpace(value) == target {
			return true
		}
	}
	return false
}

func ValidateCommandSuggestion(mode string, output string) error {
	if mode != "suggest_next_command" && mode != "write_command_from_goal" {
		return nil
	}

	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return fmt.Errorf("AI returned an empty command")
	}
	if strings.Contains(trimmed, "\n") || strings.Contains(trimmed, "\r") {
		return fmt.Errorf("AI returned multiple lines for single-command mode")
	}
	if strings.Contains(trimmed, "```") {
		return fmt.Errorf("AI returned markdown fences for single-command mode")
	}

	lower := strings.ToLower(trimmed)
	for _, fragment := range immutableBlockedCommandFragments {
		if strings.Contains(lower, fragment) {
			return fmt.Errorf("AI returned a blocked command fragment %q", fragment)
		}
	}
	for _, fragment := range []string{";", "&&", "||", "|", ">", "<"} {
		if strings.Contains(trimmed, fragment) {
			return fmt.Errorf("AI returned a chained or redirected command")
		}
	}
	if strings.Contains(lower, "curl ") && strings.Contains(lower, "sh") {
		return fmt.Errorf("AI returned a pipe-to-shell style command")
	}

	return nil
}
