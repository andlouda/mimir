// Last ~12 KB of each terminal's output, for the AI panel and templates.
// Kept outside the terminals store on purpose: updating the store on every
// output chunk re-rendered the sidebar, pane headers and panel for each one.
const OUTPUT_TAIL_MAX = 12000;
const tails = new Map(); // terminal id → string

export function rememberOutput(id, data) {
  if (!data) return;
  const prev = tails.get(id) || '';
  tails.set(id, prev.length + data.length > OUTPUT_TAIL_MAX ? (prev + data).slice(-OUTPUT_TAIL_MAX) : prev + data);
}

/** Recent output of a terminal (empty when unknown). */
export function getTerminalOutput(id) {
  return tails.get(id) || '';
}

export function forgetOutput(id) {
  tails.delete(id);
}
