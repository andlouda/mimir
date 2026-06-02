package template

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"text/template"

	"mimir/safeio"
)

// Writer is an interface for writing data to a terminal.
// This decouples template execution from the specific PTY implementation.
type Writer interface {
	Write(p []byte) (n int, err error)
}

type TemplateParameter struct {
	Name          string   `json:"name"`
	Description   string   `json:"description,omitempty"`
	Required      bool     `json:"required"`
	Type          string   `json:"type,omitempty"`
	Pattern       string   `json:"pattern,omitempty"`
	MaxLength     int      `json:"maxLength,omitempty"`
	Options       []string `json:"options,omitempty"`
	Source        string   `json:"source,omitempty"`
	DiscoveryTool string   `json:"discoveryTool,omitempty"`
}

// Template represents a template for a command.
type Template struct {
	Name         string              `json:"name"`
	Description  string              `json:"description"`
	Category     string              `json:"category,omitempty"`
	Parameters   []TemplateParameter `json:"parameters,omitempty"`
	ToolEnabled  bool                `json:"toolEnabled"`
	DangerLevel  string              `json:"dangerLevel,omitempty"`
	ToolClass    string              `json:"toolClass,omitempty"`
	Commands     map[string]string   `json:"commands"`
	Favorite     bool                `json:"favorite"`
	SourceFile   string              `json:"-"`
	UserTemplate bool                `json:"-"`
}

// Manager manages templates.
type Manager struct {
	templates         []Template
	embeddedTemplates fs.FS
	userTemplateDir   string
}

// NewManager creates a new template manager.
func NewManager(embeddedTemplates fs.FS) *Manager {
	return &Manager{
		embeddedTemplates: embeddedTemplates,
		userTemplateDir:   defaultUserTemplateDir(),
	}
}

// NewManagerWithUserDir creates a manager with an explicit writable template directory.
func NewManagerWithUserDir(embeddedTemplates fs.FS, userTemplateDir string) *Manager {
	return &Manager{
		embeddedTemplates: embeddedTemplates,
		userTemplateDir:   userTemplateDir,
	}
}

func defaultUserTemplateDir() string {
	configDir, err := os.UserConfigDir()
	if err != nil || strings.TrimSpace(configDir) == "" {
		configDir = os.TempDir()
	}
	return filepath.Join(configDir, "mimir", "templates")
}

func normalizeTemplate(tmpl *Template) {
	if strings.TrimSpace(tmpl.Category) == "" {
		tmpl.Category = "General"
	}
	if tmpl.Commands == nil {
		tmpl.Commands = map[string]string{}
	}
	if strings.TrimSpace(tmpl.DangerLevel) == "" {
		tmpl.DangerLevel = "low"
	}
	if strings.TrimSpace(tmpl.ToolClass) == "" {
		tmpl.ToolClass = deriveToolClass(tmpl.DangerLevel, tmpl.Commands)
	}
	if !tmpl.ToolEnabled && tmpl.DangerLevel != "high" {
		// For legacy templates without metadata we default to tool-enabled.
		tmpl.ToolEnabled = true
	}
	if len(tmpl.Parameters) == 0 {
		tmpl.Parameters = deriveTemplateParameters(tmpl.Commands)
	}
}

func deriveToolClass(dangerLevel string, commands map[string]string) string {
	switch strings.TrimSpace(strings.ToLower(dangerLevel)) {
	case "high":
		return "destructive"
	case "medium":
		return "mutating"
	}

	joined := strings.ToLower(strings.Join(func() []string {
		values := make([]string, 0, len(commands))
		for _, command := range commands {
			values = append(values, command)
		}
		return values
	}(), "\n"))

	switch {
	case strings.Contains(joined, "authorization:"),
		strings.Contains(joined, "openai_api_key"),
		strings.Contains(joined, "env:"),
		strings.Contains(joined, "printenv"),
		strings.Contains(joined, ".kube/config"),
		strings.Contains(joined, "private key"):
		return "secret_access"
	case strings.Contains(joined, "kubectl logs"),
		strings.Contains(joined, "docker logs"),
		strings.Contains(joined, "journalctl"),
		strings.Contains(joined, "tail "),
		strings.Contains(joined, "grep "):
		return "sensitive_readonly"
	default:
		return "safe_readonly"
	}
}

var builtinTemplateVariables = map[string]struct{}{
	"CurrentDir":   {},
	"Username":     {},
	"Hostname":     {},
	"SelectedText": {},
	"Clipboard":    {},
}

var (
	templateVariablePattern = regexp.MustCompile(`{{\s*\.(\w+)\s*}}`)
	defaultShellAtomPattern = regexp.MustCompile(`^[A-Za-z0-9_./:@%+=,\\-]+$`)
)

func deriveTemplateParameters(commands map[string]string) []TemplateParameter {
	seen := map[string]struct{}{}
	parameters := []TemplateParameter{}

	for _, command := range commands {
		matches := templateVariablePattern.FindAllStringSubmatch(command, -1)
		for _, match := range matches {
			if len(match) < 2 {
				continue
			}
			name := match[1]
			if _, ok := builtinTemplateVariables[name]; ok {
				continue
			}
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			parameters = append(parameters, TemplateParameter{
				Name:     name,
				Required: true,
			})
		}
	}

	slices.SortFunc(parameters, func(a, b TemplateParameter) int {
		return strings.Compare(a.Name, b.Name)
	})

	return parameters
}

func commandVariableNames(command string) map[string]struct{} {
	result := map[string]struct{}{}
	matches := templateVariablePattern.FindAllStringSubmatch(command, -1)
	for _, match := range matches {
		if len(match) >= 2 {
			result[match[1]] = struct{}{}
		}
	}
	return result
}

func validateTemplateInputs(tmpl Template, command string, data map[string]string) error {
	used := commandVariableNames(command)
	if len(used) == 0 {
		return nil
	}

	paramsByName := map[string]TemplateParameter{}
	for _, parameter := range tmpl.Parameters {
		paramsByName[parameter.Name] = parameter
	}

	for name := range used {
		value := data[name]
		parameter, hasParameter := paramsByName[name]

		if hasParameter {
			if parameter.Required && strings.TrimSpace(value) == "" {
				return fmt.Errorf("template parameter %s is required", name)
			}
			if parameter.MaxLength > 0 && len(value) > parameter.MaxLength {
				return fmt.Errorf("template parameter %s exceeds max length %d", name, parameter.MaxLength)
			}
			if len(parameter.Options) > 0 && !slices.Contains(parameter.Options, value) {
				return fmt.Errorf("template parameter %s is not an allowed option", name)
			}
			if strings.TrimSpace(parameter.Pattern) != "" {
				re, err := regexp.Compile(parameter.Pattern)
				if err != nil {
					return fmt.Errorf("template parameter %s has invalid validation pattern: %w", name, err)
				}
				matches := re.FindStringIndex(value)
				if matches == nil || matches[0] != 0 || matches[1] != len(value) {
					return fmt.Errorf("template parameter %s failed validation", name)
				}
				continue
			}
		}

		if value != "" && !defaultShellAtomPattern.MatchString(value) {
			return fmt.Errorf("template variable %s contains unsafe shell characters", name)
		}
	}

	return nil
}

func loadTemplatesFromFS(source fs.FS, userTemplate bool) ([]Template, error) {
	files, err := fs.ReadDir(source, "templates")
	if err != nil {
		if userTemplate && os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	loadedTemplates := []Template{}
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".json") {
			filePath := "templates/" + file.Name()

			data, err := fs.ReadFile(source, filePath)
			if err != nil {
				continue
			}

			var tmpl Template
			if err := json.Unmarshal(data, &tmpl); err != nil {
				continue
			}
			tmpl.SourceFile = file.Name()
			tmpl.UserTemplate = userTemplate
			normalizeTemplate(&tmpl)
			loadedTemplates = append(loadedTemplates, tmpl)
		}
	}
	return loadedTemplates, nil
}

// LoadTemplates loads bundled templates and user overrides from the private config dir.
func (m *Manager) LoadTemplates() error {
	loadedTemplates, err := loadTemplatesFromFS(m.embeddedTemplates, false)
	if err != nil {
		return fmt.Errorf("failed to read embedded templates directory: %w", err)
	}

	if strings.TrimSpace(m.userTemplateDir) != "" {
		userTemplates, err := loadTemplatesFromFS(os.DirFS(filepath.Dir(m.userTemplateDir)), true)
		if err != nil {
			return fmt.Errorf("failed to read user templates directory: %w", err)
		}
		loadedTemplates = append(loadedTemplates, userTemplates...)
	}

	deduped := make([]Template, 0, len(loadedTemplates))
	seenByName := make(map[string]int, len(loadedTemplates))
	for _, tmpl := range loadedTemplates {
		if existingIndex, ok := seenByName[tmpl.Name]; ok {
			deduped[existingIndex] = tmpl
			continue
		}
		seenByName[tmpl.Name] = len(deduped)
		deduped = append(deduped, tmpl)
	}

	sort.SliceStable(deduped, func(i, j int) bool {
		leftCategory := deduped[i].Category
		rightCategory := deduped[j].Category
		if leftCategory == rightCategory {
			return deduped[i].Name < deduped[j].Name
		}
		return leftCategory < rightCategory
	})

	m.templates = deduped
	return nil
}

func (m *Manager) userTemplatePath(filename string) string {
	return filepath.Join(m.userTemplateDir, filename)
}

// GetTemplates returns the loaded templates.
func (m *Manager) GetTemplates() ([]Template, error) {
	return m.templates, nil
}

// ReloadTemplates reloads the templates.
func (m *Manager) ReloadTemplates() ([]Template, error) {
	err := m.LoadTemplates()
	if err != nil {
		return nil, fmt.Errorf("failed to reload templates: %w", err)
	}
	return m.templates, nil
}

// GetToolTemplates returns templates that are safe to expose as AI-invokable tools.
func (m *Manager) GetToolTemplates() []Template {
	tools := make([]Template, 0, len(m.templates))
	for _, tmpl := range m.templates {
		if tmpl.ToolEnabled && tmpl.DangerLevel != "high" {
			tools = append(tools, tmpl)
		}
	}
	return tools
}

// TemplateContext holds the dynamic variables for template execution.
type TemplateContext struct {
	CurrentDir   string
	Username     string
	Hostname     string
	SelectedText string // Placeholder for future use
	Clipboard    string // Placeholder for future use
	Variables    map[string]string
}

// ApplyTemplate applies a template by finding the matching template,
// executing it with the provided context, and writing the result to the writer.
func (m *Manager) ApplyTemplate(id int, templateName string, terminalType string, pty Writer, ctx TemplateContext) error {
	var command string
	var selectedTemplate Template
	foundTemplate := false
	for _, t := range m.templates {
		if t.Name == templateName {
			selectedTemplate = t
			normalizeTemplate(&selectedTemplate)
			cmd, cmdOk := t.Commands[terminalType]
			if !cmdOk {
				return fmt.Errorf("template %s does not support terminal type %s", templateName, terminalType)
			}
			command = cmd
			foundTemplate = true
			break
		}
	}

	if !foundTemplate {
		return fmt.Errorf("template %s not found", templateName)
	}

	// Parse the command as a Go template to allow variable substitution
	tmpl, err := template.New("command").Parse(command)
	if err != nil {
		return fmt.Errorf("failed to parse template command: %w", err)
	}

	// Execute template with provided context variables
	data := map[string]string{
		"CurrentDir":   ctx.CurrentDir,
		"Username":     ctx.Username,
		"Hostname":     ctx.Hostname,
		"SelectedText": ctx.SelectedText,
		"Clipboard":    ctx.Clipboard,
	}
	for key, value := range ctx.Variables {
		data[key] = value
	}
	if err := validateTemplateInputs(selectedTemplate, command, data); err != nil {
		return err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("failed to execute template command: %w", err)
	}
	executedCommand := buf.String()

	// Write the executed command to the terminal with carriage return and newline
	_, err = pty.Write([]byte(executedCommand + "\r\n"))
	if err != nil {
		return fmt.Errorf("failed to write to terminal %d: %w", id, err)
	}

	return nil
}

// SaveTemplate saves a template to a file and returns updated template list.
func (m *Manager) SaveTemplate(templateJSON string) ([]Template, error) {
	var template Template
	if err := json.Unmarshal([]byte(templateJSON), &template); err != nil {
		return nil, fmt.Errorf("failed to parse template JSON: %w", err)
	}
	normalizeTemplate(&template)

	sanitizedName := m.sanitizeFilename(template.Name)
	if sanitizedName == "" {
		return nil, fmt.Errorf("template name cannot be empty or contain only invalid characters")
	}
	filename := sanitizedName + ".json"
	filePath := m.userTemplatePath(filename)

	if err := safeio.AtomicWriteFile(filePath, []byte(templateJSON), 0600); err != nil {
		return nil, fmt.Errorf("failed to write template file: %w", err)
	}

	// Reload templates
	if err := m.LoadTemplates(); err != nil {
		return nil, err
	}

	return m.templates, nil
}

// UpdateTemplate updates an existing template file and returns updated template list.
func (m *Manager) UpdateTemplate(templateJSON string) ([]Template, error) {
	var template Template
	if err := json.Unmarshal([]byte(templateJSON), &template); err != nil {
		return nil, fmt.Errorf("failed to parse template JSON: %w", err)
	}
	normalizeTemplate(&template)

	sanitizedName := m.sanitizeFilename(template.Name)
	if sanitizedName == "" {
		return nil, fmt.Errorf("template name cannot be empty or contain only invalid characters")
	}
	filename := sanitizedName + ".json"
	filePath := m.userTemplatePath(filename)

	if err := safeio.AtomicWriteFile(filePath, []byte(templateJSON), 0600); err != nil {
		return nil, fmt.Errorf("failed to update template file: %w", err)
	}

	// Reload templates
	if err := m.LoadTemplates(); err != nil {
		return nil, err
	}

	return m.templates, nil
}

// DeleteTemplate deletes a template file and returns updated template list.
func (m *Manager) DeleteTemplate(templateName string) ([]Template, error) {
	sanitizedName := m.sanitizeFilename(templateName)
	if sanitizedName == "" {
		return nil, fmt.Errorf("template name cannot be empty or contain only invalid characters")
	}
	filename := sanitizedName + ".json"
	filePath := m.userTemplatePath(filename)

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("template %s is bundled or does not exist as a user template", templateName)
	}

	if err := os.Remove(filePath); err != nil {
		return nil, fmt.Errorf("failed to delete template file %s: %w", filePath, err)
	}

	// Reload templates
	if err := m.LoadTemplates(); err != nil {
		return nil, err
	}

	return m.templates, nil
}

// ToggleFavorite toggles the favorite status of a template and returns updated template list.
func (m *Manager) ToggleFavorite(templateName string) ([]Template, error) {
	// Find the template
	var foundTemplate *Template
	for i := range m.templates {
		if m.templates[i].Name == templateName {
			foundTemplate = &m.templates[i]
			break
		}
	}

	if foundTemplate == nil {
		return nil, fmt.Errorf("template %s not found", templateName)
	}

	// Toggle favorite status
	foundTemplate.Favorite = !foundTemplate.Favorite

	// Save updated template to file
	templateJSON, err := json.MarshalIndent(foundTemplate, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal template: %w", err)
	}

	filename := foundTemplate.SourceFile
	if strings.TrimSpace(filename) == "" {
		sanitizedName := m.sanitizeFilename(foundTemplate.Name)
		filename = sanitizedName + ".json"
	}
	filePath := m.userTemplatePath(filename)

	if err := safeio.AtomicWriteFile(filePath, templateJSON, 0600); err != nil {
		return nil, fmt.Errorf("failed to write template file: %w", err)
	}

	// Reload templates
	if err := m.LoadTemplates(); err != nil {
		return nil, err
	}

	return m.templates, nil
}

// sanitizeFilename removes invalid characters from filenames to prevent
// path traversal attacks and ensure cross-platform compatibility.
func (m *Manager) sanitizeFilename(name string) string {
	name = strings.ReplaceAll(name, " ", "_")
	reg := regexp.MustCompile(`[^a-zA-Z0-9_-]+`)
	name = reg.ReplaceAllString(name, "")
	name = strings.ReplaceAll(name, "/", "")
	name = strings.ReplaceAll(name, "\\", "")
	return name
}
