package tools

import (
	"bytes"
	"io/fs"
	"testing"
	"testing/fstest"

	"mimir/template"
)

type mockWriter struct {
	buf bytes.Buffer
}

func (m *mockWriter) Write(p []byte) (int, error) {
	return m.buf.Write(p)
}

func createTemplateToolTestFS() fs.FS {
	return fstest.MapFS{
		"templates/k8s_logs.json": &fstest.MapFile{
			Data: []byte(`{
				"name": "K8s Pod Logs",
				"description": "Show logs for a pod.",
				"category": "Kubernetes",
				"dangerLevel": "medium",
				"toolEnabled": true,
				"commands": {
					"bash": "kubectl logs -n {{.Namespace}} {{.Pod}}"
				}
			}`),
		},
	}
}

func TestTemplateToolRun(t *testing.T) {
	manager := template.NewManager(createTemplateToolTestFS())
	if err := manager.LoadTemplates(); err != nil {
		t.Fatalf("load templates: %v", err)
	}

	templates := manager.GetToolTemplates()
	if len(templates) != 1 {
		t.Fatalf("expected 1 tool template, got %d", len(templates))
	}

	tool := NewTemplateTool(manager, templates[0])
	writer := &mockWriter{}
	result, err := tool.Run(RunContext{
		TerminalID:   7,
		TerminalType: "bash",
		Writer:       writer,
	}, map[string]string{
		"Namespace": "default",
		"Pod":       "api-123",
	})
	if err != nil {
		t.Fatalf("run template tool: %v", err)
	}

	if got := writer.buf.String(); got != "kubectl logs -n default api-123\r\n" {
		t.Fatalf("unexpected terminal output: %q", got)
	}
	if result.Metadata["template_name"] != "K8s Pod Logs" {
		t.Fatalf("unexpected metadata: %+v", result.Metadata)
	}
	if tool.Risk() != RiskMedium {
		t.Fatalf("expected medium risk, got %s", tool.Risk())
	}
	if tool.Class() != ClassMutating {
		t.Fatalf("expected mutating class, got %s", tool.Class())
	}
	if len(tool.Parameters()) != 2 {
		t.Fatalf("expected derived parameters, got %+v", tool.Parameters())
	}
}

func TestBuildTemplateTools(t *testing.T) {
	manager := template.NewManager(createTemplateToolTestFS())
	if err := manager.LoadTemplates(); err != nil {
		t.Fatalf("load templates: %v", err)
	}

	tools := BuildTemplateTools(manager, manager.GetToolTemplates())
	if len(tools) != 1 {
		t.Fatalf("expected 1 built tool, got %d", len(tools))
	}
	if tools[0].ID() != "template:K8s Pod Logs" {
		t.Fatalf("unexpected tool ID: %s", tools[0].ID())
	}
}
