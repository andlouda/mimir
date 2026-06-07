// Pure helpers for command templates. State-free; safe to unit-test.

// Variables provided automatically by the backend template context — these are
// never prompted for.
const builtinTemplateVariables = new Set(['CurrentDir', 'Username', 'Hostname', 'SelectedText', 'Clipboard']);

/** Fills in defaults for a (possibly partial) template object. */
export function normalizeTemplate(template) {
  return {
    ...template,
    name: template?.name || 'Unnamed Template',
    description: template?.description || '',
    category: template?.category || 'General',
    commands: template?.commands || {},
    parameters: Array.isArray(template?.parameters) ? template.parameters : [],
    toolEnabled: template?.toolEnabled ?? true,
    dangerLevel: template?.dangerLevel || 'low',
    favorite: Boolean(template?.favorite),
  };
}

/** Normalizes an array of templates; non-arrays become []. */
export function normalizeTemplates(nextTemplates) {
  if (!Array.isArray(nextTemplates)) {
    return [];
  }
  return nextTemplates.map(normalizeTemplate);
}

/** Extracts user-fillable `{{ .Var }}` names from a command, skipping built-ins and duplicates. */
export function extractTemplateVariables(command) {
  const matches = command.matchAll(/{{\s*\.(\w+)\s*}}/g);
  const variables = [];
  const seen = new Set();
  for (const match of matches) {
    const variableName = match[1];
    if (builtinTemplateVariables.has(variableName) || seen.has(variableName)) {
      continue;
    }
    seen.add(variableName);
    variables.push(variableName);
  }
  return variables;
}

/** Turns a camelCase variable name into a spaced prompt label. */
export function buildPromptLabel(variableName) {
  return variableName.replace(/([a-z0-9])([A-Z])/g, '$1 $2');
}
