package tools

import (
	"fmt"
	"strings"

	"mimir/template"
)

// TemplateTool adapts a template.Template to the generic Tool interface.
type TemplateTool struct {
	manager   *template.Manager
	template  template.Template
	toolID    string
	params    []Parameter
	riskLevel RiskLevel
	class     ToolClass
}

// NewTemplateTool creates a Tool adapter for a template-backed command.
func NewTemplateTool(manager *template.Manager, tmpl template.Template) *TemplateTool {
	params := make([]Parameter, 0, len(tmpl.Parameters))
	for _, param := range tmpl.Parameters {
		params = append(params, Parameter{
			Name:          param.Name,
			Description:   param.Description,
			Required:      param.Required,
			Type:          param.Type,
			Pattern:       param.Pattern,
			MaxLength:     param.MaxLength,
			Options:       append([]string(nil), param.Options...),
			Source:        ParameterSource(param.Source),
			DiscoveryTool: param.DiscoveryTool,
		})
	}

	return &TemplateTool{
		manager:   manager,
		template:  tmpl,
		toolID:    "template:" + tmpl.Name,
		params:    params,
		riskLevel: mapRiskLevel(tmpl.DangerLevel),
		class:     mapToolClass(tmpl.ToolClass, tmpl.DangerLevel),
	}
}

func mapRiskLevel(level string) RiskLevel {
	switch strings.TrimSpace(strings.ToLower(level)) {
	case string(RiskMedium):
		return RiskMedium
	case string(RiskHigh):
		return RiskHigh
	default:
		return RiskLow
	}
}

func mapToolClass(className string, dangerLevel string) ToolClass {
	switch strings.TrimSpace(strings.ToLower(className)) {
	case string(ClassSensitiveReadonly):
		return ClassSensitiveReadonly
	case string(ClassMutating):
		return ClassMutating
	case string(ClassDestructive):
		return ClassDestructive
	case string(ClassSecretAccess):
		return ClassSecretAccess
	case string(ClassSafeReadonly):
		return ClassSafeReadonly
	}

	switch strings.TrimSpace(strings.ToLower(dangerLevel)) {
	case string(RiskHigh):
		return ClassDestructive
	case string(RiskMedium):
		return ClassMutating
	default:
		return ClassSafeReadonly
	}
}

func (t *TemplateTool) ID() string {
	return t.toolID
}

func (t *TemplateTool) Name() string {
	return t.template.Name
}

func (t *TemplateTool) Description() string {
	return t.template.Description
}

func (t *TemplateTool) Category() string {
	return t.template.Category
}

func (t *TemplateTool) Risk() RiskLevel {
	return t.riskLevel
}

func (t *TemplateTool) Class() ToolClass {
	return t.class
}

func (t *TemplateTool) Parameters() []Parameter {
	return append([]Parameter(nil), t.params...)
}

func (t *TemplateTool) CommandMap() map[string]string {
	result := make(map[string]string, len(t.template.Commands))
	for key, value := range t.template.Commands {
		result[key] = value
	}
	return result
}

func (t *TemplateTool) Run(ctx RunContext, input map[string]string) (ToolResult, error) {
	if t.manager == nil {
		return ToolResult{}, fmt.Errorf("template manager is required")
	}
	if ctx.Writer == nil {
		return ToolResult{}, fmt.Errorf("writer is required")
	}
	if strings.TrimSpace(ctx.TerminalType) == "" {
		return ToolResult{}, fmt.Errorf("terminal type is required")
	}

	templateCtx := ctx.TemplateData
	if len(input) > 0 {
		templateCtx.Variables = input
	}

	if err := t.manager.ApplyTemplate(ctx.TerminalID, t.template.Name, ctx.TerminalType, ctx.Writer, templateCtx); err != nil {
		return ToolResult{}, err
	}

	return ToolResult{
		Output: fmt.Sprintf("Executed template %s", t.template.Name),
		Metadata: map[string]string{
			"tool_id":       t.toolID,
			"template_name": t.template.Name,
			"category":      t.template.Category,
		},
	}, nil
}

// BuildTemplateTools adapts a set of template tools into Tool values.
func BuildTemplateTools(manager *template.Manager, templates []template.Template) []Tool {
	result := make([]Tool, 0, len(templates))
	for _, tmpl := range templates {
		result = append(result, NewTemplateTool(manager, tmpl))
	}
	return result
}
