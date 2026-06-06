package workflow

import (
	"os"
	"path/filepath"
	"testing"

	"mimir/template"
	"mimir/tools"
)

func TestDefaultPlaybooks(t *testing.T) {
	playbooks := DefaultPlaybooks()
	if len(playbooks) == 0 {
		t.Fatal("expected default playbooks")
	}

	seen := make(map[string]struct{}, len(playbooks))
	for _, playbook := range playbooks {
		if playbook.ID == "" {
			t.Fatal("expected playbook ID")
		}
		if _, ok := seen[playbook.ID]; ok {
			t.Fatalf("duplicate playbook id %s", playbook.ID)
		}
		seen[playbook.ID] = struct{}{}
		if len(playbook.Steps) == 0 {
			t.Fatalf("expected playbook %s to have steps", playbook.ID)
		}
		if playbook.Name == "" {
			t.Fatalf("expected playbook %s to have a name", playbook.ID)
		}
		if err := ValidateDefinition(playbook); err != nil {
			t.Fatalf("expected playbook %s to validate: %v", playbook.ID, err)
		}
	}
}

func TestDefaultPlaybooksReferenceRegisteredReadonlyTools(t *testing.T) {
	registry := registryFromBundledTemplates(t)
	discoveryTools := map[string]struct{}{
		"discovery:list_k8s_namespaces":    {},
		"discovery:list_k8s_pods":          {},
		"discovery:list_k8s_resources":     {},
		"discovery:list_docker_containers": {},
		"discovery:list_compose_services":  {},
	}
	allowedClasses := map[tools.ToolClass]struct{}{
		tools.ClassSafeReadonly:      {},
		tools.ClassSensitiveReadonly: {},
	}

	for _, playbook := range DefaultPlaybooks() {
		if err := ValidateDefinition(playbook); err != nil {
			t.Fatalf("playbook %s should validate before registry checks: %v", playbook.ID, err)
		}

		for _, step := range playbook.Steps {
			switch step.Type {
			case StepRunTool:
				tool, ok := registry.Get(step.Tool)
				if !ok {
					t.Fatalf("playbook %s step %s references unknown or disabled tool %q", playbook.ID, step.ID, step.Tool)
				}
				if _, ok := allowedClasses[tool.Class()]; !ok {
					t.Fatalf("playbook %s step %s references non-readonly tool %q class=%s risk=%s", playbook.ID, step.ID, tool.ID(), tool.Class(), tool.Risk())
				}
				if tool.Risk() == tools.RiskHigh {
					t.Fatalf("playbook %s step %s references high-risk tool %q", playbook.ID, step.ID, tool.ID())
				}
				for _, parameter := range tool.Parameters() {
					if parameter.Source != tools.ParameterSourceDiscoveryOnly {
						continue
					}
					if _, ok := discoveryTools[parameter.DiscoveryTool]; !ok {
						t.Fatalf("playbook %s step %s tool %q parameter %s references unknown discovery tool %q", playbook.ID, step.ID, tool.ID(), parameter.Name, parameter.DiscoveryTool)
					}
				}
			case StepRunDiscovery:
				if _, ok := discoveryTools[step.DiscoveryTool]; !ok {
					t.Fatalf("playbook %s step %s references unknown discovery tool %q", playbook.ID, step.ID, step.DiscoveryTool)
				}
			case StepAskAI, StepAskUser:
				// Schema validation above covers prompt requirements.
			default:
				t.Fatalf("playbook %s step %s uses unsupported step type %q", playbook.ID, step.ID, step.Type)
			}
		}
	}
}

func registryFromBundledTemplates(t *testing.T) *tools.Registry {
	t.Helper()

	userTemplateDir := filepath.Join(t.TempDir(), "templates")
	if err := os.MkdirAll(userTemplateDir, 0o700); err != nil {
		t.Fatalf("create temp user template dir: %v", err)
	}

	manager := template.NewManagerWithUserDir(os.DirFS(".."), userTemplateDir)
	if err := manager.LoadTemplates(); err != nil {
		t.Fatalf("load bundled templates: %v", err)
	}

	registry := tools.NewRegistry()
	for _, tool := range tools.BuildTemplateTools(manager, manager.GetToolTemplates()) {
		if err := registry.Register(tool); err != nil {
			t.Fatalf("register template tool %s: %v", tool.ID(), err)
		}
	}
	return registry
}
