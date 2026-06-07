// Pure helpers for grouping terminal sessions in the sidebar.
// Extracted from App.svelte: these are state-free and depend only on their
// arguments, which keeps them unit-testable and shrinks the root component.

/** Maps a terminal type to its built-in auto-group id. */
export function terminalGroupFor(type) {
  if (type === 'ssh') return 'ssh';
  if (['bash', 'zsh', 'wsl'].includes(type)) return 'local';
  if (['cmd', 'powershell'].includes(type)) return 'windows';
  return 'other';
}

/** Human-readable label for an auto-group id. */
export function terminalGroupLabel(group) {
  return {
    local: 'Local',
    ssh: 'SSH',
    windows: 'Windows',
    other: 'Other',
  }[group] || group;
}

/**
 * Builds the ordered list of sidebar groups: custom folders first (by
 * position), then non-empty built-in auto-groups. Pure function of its inputs.
 */
export function groupedSidebarTerminals(terminalsList, folders) {
  // Custom folders first (sorted by position)
  const folderGroups = [...folders]
    .sort((a, b) => a.position - b.position)
    .map((f) => ({ id: `folder:${f.id}`, label: f.name, terminals: [], isCustom: true }));
  const folderByID = new Map(folderGroups.map((g) => [g.id, g]));

  // Built-in auto-groups
  const autoGroups = ['local', 'ssh', 'windows', 'other'].map((id) => ({ id, label: terminalGroupLabel(id), terminals: [], isCustom: false }));
  const autoByID = new Map(autoGroups.map((g) => [g.id, g]));

  for (const terminal of terminalsList) {
    if (terminal.folderId && folderByID.has(`folder:${terminal.folderId}`)) {
      folderByID.get(`folder:${terminal.folderId}`).terminals.push(terminal);
    } else {
      const group = autoByID.get(terminalGroupFor(terminal.type)) || autoByID.get('other');
      group.terminals.push(terminal);
    }
  }

  return [...folderGroups.filter((g) => g.isCustom || g.terminals.length > 0), ...autoGroups.filter((g) => g.terminals.length > 0)];
}
