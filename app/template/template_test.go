package template

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

// mockWriter implements Writer for testing.
type mockWriter struct {
	buf bytes.Buffer
}

func (m *mockWriter) Write(p []byte) (int, error) {
	return m.buf.Write(p)
}

func createTestFS() fs.FS {
	return fstest.MapFS{
		"templates/test_list.json": &fstest.MapFile{
			Data: []byte(`{
				"name": "List Files",
				"description": "Lists files",
				"category": "Files",
				"commands": {
					"bash": "ls -la",
					"cmd": "dir",
					"powershell": "Get-ChildItem"
				},
				"favorite": false
			}`),
		},
		"templates/test_cd.json": &fstest.MapFile{
			Data: []byte(`{
				"name": "Change Dir",
				"description": "Change directory using template variables",
				"category": "Navigation",
				"commands": {
					"bash": "cd {{.CurrentDir}}",
					"cmd": "cd /d {{.CurrentDir}}"
				},
				"favorite": true
			}`),
		},
	}
}

func createTestManager() *Manager {
	m := NewManager(createTestFS())
	m.LoadTemplates()
	return m
}

func TestLoadTemplates(t *testing.T) {
	m := createTestManager()

	templates, err := m.GetTemplates()
	if err != nil {
		t.Fatalf("GetTemplates failed: %v", err)
	}
	if len(templates) != 2 {
		t.Fatalf("Expected 2 templates, got %d", len(templates))
	}
	if templates[0].Category == "" {
		t.Fatal("expected category to be loaded")
	}
}

func TestReloadTemplates(t *testing.T) {
	m := createTestManager()

	templates, err := m.ReloadTemplates()
	if err != nil {
		t.Fatalf("ReloadTemplates failed: %v", err)
	}
	if len(templates) != 2 {
		t.Fatalf("Expected 2 templates, got %d", len(templates))
	}
}

func TestApplyTemplate(t *testing.T) {
	m := createTestManager()
	w := &mockWriter{}

	err := m.ApplyTemplate(1, "List Files", "bash", w, TemplateContext{})
	if err != nil {
		t.Fatalf("ApplyTemplate failed: %v", err)
	}
	written := w.buf.String()
	if written != "ls -la\r\n" {
		t.Errorf("Expected 'ls -la\\r\\n', got %q", written)
	}
}

func TestApplyTemplateWithVariables(t *testing.T) {
	m := createTestManager()
	w := &mockWriter{}

	ctx := TemplateContext{
		CurrentDir: "/home/user",
		Username:   "testuser",
		Hostname:   "testhost",
	}

	err := m.ApplyTemplate(1, "Change Dir", "bash", w, ctx)
	if err != nil {
		t.Fatalf("ApplyTemplate failed: %v", err)
	}
	written := w.buf.String()
	expected := "cd /home/user\r\n"
	if written != expected {
		t.Errorf("Expected %q, got %q", expected, written)
	}
}

func TestApplyTemplateWithCustomVariables(t *testing.T) {
	m := &Manager{
		templates: []Template{
			{
				Name:        "Kubectl Logs",
				Description: "Logs by namespace and pod",
				Commands: map[string]string{
					"bash": "kubectl logs -n {{.Namespace}} {{.Pod}}",
				},
			},
		},
	}
	w := &mockWriter{}

	err := m.ApplyTemplate(1, "Kubectl Logs", "bash", w, TemplateContext{
		Variables: map[string]string{
			"Namespace": "default",
			"Pod":       "api-123",
		},
	})
	if err != nil {
		t.Fatalf("ApplyTemplate failed: %v", err)
	}

	if written := w.buf.String(); written != "kubectl logs -n default api-123\r\n" {
		t.Fatalf("unexpected command output: %q", written)
	}
}

func TestApplyTemplateRejectsUnsafeShellVariable(t *testing.T) {
	m := &Manager{
		templates: []Template{
			{
				Name:        "Kubectl Logs",
				Description: "Logs by namespace and pod",
				Commands: map[string]string{
					"bash": "kubectl logs -n {{.Namespace}} {{.Pod}}",
				},
			},
		},
	}
	w := &mockWriter{}

	err := m.ApplyTemplate(1, "Kubectl Logs", "bash", w, TemplateContext{
		Variables: map[string]string{
			"Namespace": "default",
			"Pod":       "api-123; rm -rf ~/.config/mimir",
		},
	})
	if err == nil {
		t.Fatal("expected unsafe shell variable to be rejected")
	}
	if !strings.Contains(err.Error(), "unsafe shell characters") {
		t.Fatalf("unexpected error: %v", err)
	}
	if w.buf.Len() != 0 {
		t.Fatalf("unsafe command was written: %q", w.buf.String())
	}
}

func TestApplyTemplateEnforcesParameterPattern(t *testing.T) {
	m := &Manager{
		templates: []Template{
			{
				Name:        "Docker Logs",
				Description: "Logs by service",
				Parameters: []TemplateParameter{
					{
						Name:      "Service",
						Required:  true,
						Pattern:   "^[a-zA-Z0-9][a-zA-Z0-9_.-]*$",
						MaxLength: 128,
					},
				},
				Commands: map[string]string{
					"bash": "docker compose logs -f {{.Service}}",
				},
			},
		},
	}
	w := &mockWriter{}

	err := m.ApplyTemplate(1, "Docker Logs", "bash", w, TemplateContext{
		Variables: map[string]string{"Service": "api;whoami"},
	})
	if err == nil {
		t.Fatal("expected parameter pattern violation")
	}
	if !strings.Contains(err.Error(), "failed validation") {
		t.Fatalf("unexpected error: %v", err)
	}

	err = m.ApplyTemplate(1, "Docker Logs", "bash", w, TemplateContext{
		Variables: map[string]string{"Service": "api-1"},
	})
	if err != nil {
		t.Fatalf("expected safe parameter to pass: %v", err)
	}
	if written := w.buf.String(); written != "docker compose logs -f api-1\r\n" {
		t.Fatalf("unexpected command output: %q", written)
	}
}

func TestApplyTemplateRequiresFullParameterPatternMatch(t *testing.T) {
	m := &Manager{
		templates: []Template{
			{
				Name:        "Service Logs",
				Description: "Logs by service",
				Parameters: []TemplateParameter{
					{
						Name:     "Service",
						Required: true,
						Pattern:  "api",
					},
				},
				Commands: map[string]string{
					"bash": "docker compose logs {{.Service}}",
				},
			},
		},
	}
	w := &mockWriter{}

	err := m.ApplyTemplate(1, "Service Logs", "bash", w, TemplateContext{
		Variables: map[string]string{"Service": "api;whoami"},
	})
	if err == nil {
		t.Fatal("expected partial pattern match to be rejected")
	}
	if !strings.Contains(err.Error(), "failed validation") {
		t.Fatalf("unexpected error: %v", err)
	}
	if w.buf.Len() != 0 {
		t.Fatalf("unsafe command was written: %q", w.buf.String())
	}

	err = m.ApplyTemplate(1, "Service Logs", "bash", w, TemplateContext{
		Variables: map[string]string{"Service": "api"},
	})
	if err != nil {
		t.Fatalf("expected exact pattern match to pass: %v", err)
	}
}

func TestApplyTemplateNotFound(t *testing.T) {
	m := createTestManager()
	w := &mockWriter{}

	err := m.ApplyTemplate(1, "NonExistent", "bash", w, TemplateContext{})
	if err == nil {
		t.Fatal("Expected error for non-existent template")
	}
}

func TestApplyTemplateUnsupportedType(t *testing.T) {
	m := createTestManager()
	w := &mockWriter{}

	err := m.ApplyTemplate(1, "List Files", "zsh", w, TemplateContext{})
	if err == nil {
		t.Fatal("Expected error for unsupported terminal type")
	}
}

func TestSanitizeFilename(t *testing.T) {
	m := &Manager{}
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "simple"},
		{"with spaces", "with_spaces"},
		{"with/slashes", "withslashes"},
		{"with\\backslash", "withbackslash"},
		{"special!@#chars", "specialchars"},
		{"dashes-ok", "dashes-ok"},
		{"under_score", "under_score"},
		{"", ""},
		{"123numeric", "123numeric"},
		{"MixedCase", "MixedCase"},
		{"../../../etc/passwd", "etcpasswd"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := m.sanitizeFilename(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeFilename(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNormalizeTemplateDefaultsCategory(t *testing.T) {
	tmpl := Template{}
	normalizeTemplate(&tmpl)

	if tmpl.Category != "General" {
		t.Fatalf("expected default category General, got %q", tmpl.Category)
	}
	if tmpl.Commands == nil {
		t.Fatal("expected commands map to be initialized")
	}
	if !tmpl.ToolEnabled {
		t.Fatal("expected tool enabled by default")
	}
}

func TestDeriveTemplateParameters(t *testing.T) {
	parameters := deriveTemplateParameters(map[string]string{
		"bash": "kubectl logs -n {{.Namespace}} {{.Pod}} --container {{.Container}}",
		"cmd":  "echo {{.Namespace}}",
	})

	if len(parameters) != 3 {
		t.Fatalf("expected 3 parameters, got %d", len(parameters))
	}
	if parameters[0].Name != "Container" || parameters[1].Name != "Namespace" || parameters[2].Name != "Pod" {
		t.Fatalf("unexpected parameters: %+v", parameters)
	}
}

func TestGetToolTemplatesFiltersHighDanger(t *testing.T) {
	m := &Manager{
		templates: []Template{
			{Name: "Safe", ToolEnabled: true, DangerLevel: "low"},
			{Name: "Danger", ToolEnabled: false, DangerLevel: "high"},
		},
	}

	tools := m.GetToolTemplates()
	if len(tools) != 1 || tools[0].Name != "Safe" {
		t.Fatalf("unexpected tool list: %+v", tools)
	}
}

func TestToggleFavoriteUsesSourceFile(t *testing.T) {
	tmpDir := t.TempDir()
	templatesDir := filepath.Join(tmpDir, "templates")
	if err := os.MkdirAll(templatesDir, 0700); err != nil {
		t.Fatalf("mkdir templates dir: %v", err)
	}

	templatePath := filepath.Join(templatesDir, "prompt_current_folder.json")
	content := `{
  "name": "Prompt Current Folder Only",
  "description": "Prompt template",
  "category": "Navigation",
  "commands": {
    "bash": "export PS1='\\W \\\\$ '"
  },
  "favorite": true
}`
	if err := os.WriteFile(templatePath, []byte(content), 0600); err != nil {
		t.Fatalf("write template file: %v", err)
	}

	manager := NewManagerWithUserDir(createTestFS(), templatesDir)
	if err := manager.LoadTemplates(); err != nil {
		t.Fatalf("load templates: %v", err)
	}

	if _, err := manager.ToggleFavorite("Prompt Current Folder Only"); err != nil {
		t.Fatalf("toggle favorite: %v", err)
	}

	updated, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatalf("read template file: %v", err)
	}
	if !bytes.Contains(updated, []byte(`"favorite": false`)) {
		t.Fatalf("expected favorite to be written to source file, got: %s", string(updated))
	}
}
