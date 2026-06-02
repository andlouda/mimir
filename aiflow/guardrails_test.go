package aiflow

import (
	"testing"

	"mimir/template"
	"mimir/tools"
)

func TestTemplateAllowedForAIStrictGuardrails(t *testing.T) {
	t.Run("blocks non-low risk templates", func(t *testing.T) {
		allowed := TemplateAllowedForAI(template.Template{
			Name:        "Restart Service",
			DangerLevel: "medium",
			Commands: map[string]string{
				"bash": "systemctl restart nginx",
			},
		})
		if allowed {
			t.Fatalf("expected medium-risk template to be blocked")
		}
	})

	t.Run("blocks destructive command fragments", func(t *testing.T) {
		allowed := TemplateAllowedForAI(template.Template{
			Name:        "Docker Restart",
			DangerLevel: "low",
			Commands: map[string]string{
				"bash": "docker restart api",
			},
		})
		if allowed {
			t.Fatalf("expected destructive command fragment to be blocked")
		}
	})

	t.Run("allows read only diagnostics", func(t *testing.T) {
		allowed := TemplateAllowedForAI(template.Template{
			Name:        "Disk Usage Summary",
			DangerLevel: "low",
			Commands: map[string]string{
				"bash": "df -h",
			},
		})
		if !allowed {
			t.Fatalf("expected read-only diagnostic template to be allowed")
		}
	})
}

type inspectableStubTool struct {
	id         string
	risk       tools.RiskLevel
	class      tools.ToolClass
	parameters []tools.Parameter
	commands   map[string]string
}

type discoveryResolverStub struct {
	values map[string][]string
	err    error
}

func (s discoveryResolverStub) Resolve(discoveryTool string, _ string, _ map[string]string) ([]string, error) {
	if s.err != nil {
		return nil, s.err
	}
	return append([]string(nil), s.values[discoveryTool]...), nil
}

func (s inspectableStubTool) ID() string                    { return s.id }
func (s inspectableStubTool) Name() string                  { return s.id }
func (s inspectableStubTool) Description() string           { return "" }
func (s inspectableStubTool) Category() string              { return "Diagnostics" }
func (s inspectableStubTool) Risk() tools.RiskLevel         { return s.risk }
func (s inspectableStubTool) Class() tools.ToolClass        { return s.class }
func (s inspectableStubTool) Parameters() []tools.Parameter { return s.parameters }
func (s inspectableStubTool) Run(tools.RunContext, map[string]string) (tools.ToolResult, error) {
	return tools.ToolResult{}, nil
}
func (s inspectableStubTool) CommandMap() map[string]string { return s.commands }

func TestValidateSelectedTool(t *testing.T) {
	t.Run("rejects non-low-risk tool", func(t *testing.T) {
		err := ValidateSelectedTool(inspectableStubTool{
			id:    "template:restart",
			risk:  tools.RiskMedium,
			class: tools.ClassMutating,
			commands: map[string]string{
				"bash": "systemctl restart nginx",
			},
		}, nil)
		if err == nil {
			t.Fatalf("expected validation error")
		}
	})

	t.Run("rejects unknown parameter", func(t *testing.T) {
		err := ValidateSelectedTool(inspectableStubTool{
			id:    "template:logs",
			risk:  tools.RiskLow,
			class: tools.ClassSensitiveReadonly,
			parameters: []tools.Parameter{
				{Name: "Pod", Required: true},
			},
			commands: map[string]string{
				"bash": "kubectl logs {{.Pod}}",
			},
		}, map[string]string{"Other": "value"})
		if err == nil {
			t.Fatalf("expected validation error")
		}
	})

	t.Run("rejects suspicious parameter value", func(t *testing.T) {
		err := ValidateSelectedTool(inspectableStubTool{
			id:    "template:logs",
			risk:  tools.RiskLow,
			class: tools.ClassSensitiveReadonly,
			parameters: []tools.Parameter{
				{Name: "Pod", Required: true},
			},
			commands: map[string]string{
				"bash": "kubectl logs {{.Pod}}",
			},
		}, map[string]string{"Pod": "api; rm -rf /"})
		if err == nil {
			t.Fatalf("expected validation error")
		}
	})

	t.Run("accepts safe deterministic selection", func(t *testing.T) {
		err := ValidateSelectedTool(inspectableStubTool{
			id:    "template:logs",
			risk:  tools.RiskLow,
			class: tools.ClassSensitiveReadonly,
			parameters: []tools.Parameter{
				{Name: "Pod", Required: true},
			},
			commands: map[string]string{
				"bash": "kubectl logs {{.Pod}}",
			},
		}, map[string]string{"Pod": "api-7d4c8"})
		if err != nil {
			t.Fatalf("expected safe selection to pass, got %v", err)
		}
	})

	t.Run("rejects user-only parameter", func(t *testing.T) {
		err := ValidateSelectedTool(inspectableStubTool{
			id:    "template:service-details",
			risk:  tools.RiskLow,
			class: tools.ClassSafeReadonly,
			parameters: []tools.Parameter{
				{Name: "ServiceName", Required: true, Source: tools.ParameterSourceUserOnly},
			},
			commands: map[string]string{
				"bash": "systemctl status {{.ServiceName}}",
			},
		}, map[string]string{"ServiceName": "nginx"})
		if err == nil {
			t.Fatalf("expected validation error")
		}
	})

	t.Run("rejects discovery-only parameter without discovery", func(t *testing.T) {
		err := ValidateSelectedTool(inspectableStubTool{
			id:    "template:k8s-logs",
			risk:  tools.RiskLow,
			class: tools.ClassSensitiveReadonly,
			parameters: []tools.Parameter{
				{Name: "Pod", Required: true, Source: tools.ParameterSourceDiscoveryOnly, DiscoveryTool: "discovery:list_k8s_pods"},
			},
			commands: map[string]string{
				"bash": "kubectl logs {{.Pod}}",
			},
		}, map[string]string{"Pod": "api-123"})
		if err == nil {
			t.Fatalf("expected validation error")
		}
	})

	t.Run("accepts discovery-only parameter from resolver", func(t *testing.T) {
		err := ValidateSelectedToolWithDiscovery(inspectableStubTool{
			id:    "template:k8s-logs",
			risk:  tools.RiskLow,
			class: tools.ClassSensitiveReadonly,
			parameters: []tools.Parameter{
				{Name: "Namespace", Required: true, Source: tools.ParameterSourceDiscoveryOnly, DiscoveryTool: "discovery:list_k8s_namespaces"},
				{Name: "Pod", Required: true, Source: tools.ParameterSourceDiscoveryOnly, DiscoveryTool: "discovery:list_k8s_pods"},
			},
			commands: map[string]string{
				"bash": "kubectl logs -n {{.Namespace}} {{.Pod}}",
			},
		}, map[string]string{
			"Namespace": "default",
			"Pod":       "api-123",
		}, "bash", discoveryResolverStub{
			values: map[string][]string{
				"discovery:list_k8s_namespaces": {"default", "kube-system"},
				"discovery:list_k8s_pods":       {"api-123", "worker-1"},
			},
		})
		if err != nil {
			t.Fatalf("expected discovery-backed selection to pass, got %v", err)
		}
	})
}

func TestValidateCommandSuggestion(t *testing.T) {
	if err := ValidateCommandSuggestion("suggest_next_command", "kubectl get pods"); err != nil {
		t.Fatalf("expected safe command to pass, got %v", err)
	}
	if err := ValidateCommandSuggestion("suggest_next_command", "kubectl delete pod api"); err == nil {
		t.Fatalf("expected destructive command to be blocked")
	}
	if err := ValidateCommandSuggestion("write_command_from_goal", "echo ok && reboot"); err == nil {
		t.Fatalf("expected chained command to be blocked")
	}
}
