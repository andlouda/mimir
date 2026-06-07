// Thin adapter over the Wails transcript bindings. Centralises:
//   - the dynamic dispatch into window.go.main.App (the project still pulls
//     binding regeneration into separate PRs; this lets us call new methods
//     without rebuilding wailsjs/go/main/App.js by hand)
//   - safe defaults so callers don't have to nullcheck a missing backend
//     during dev mode / smoke tests
//
// Anything else in the codebase that talks transcripts should go through here.

function backend() {
  return typeof window !== 'undefined' ? window?.go?.main?.App : undefined;
}

/**
 * @returns {Promise<Array<{
 *   resumeId: string,
 *   name?: string,
 *   type?: string,
 *   sshProfileId?: string,
 *   size: number,
 *   modTime: string,
 * }>>}
 */
export async function listTranscripts() {
  const api = backend();
  if (!api?.ListTranscripts) return [];
  const list = await api.ListTranscripts();
  return Array.isArray(list) ? list : [];
}

/**
 * Read the full transcript for a resume id.
 *
 * Prefer getTranscriptContent in new code — it returns Truncated/Size info
 * the UI needs to draw an accurate banner. This wrapper stays for the
 * existing restore-overlay excerpt path.
 *
 * @param {string} resumeId
 * @param {number} [maxBytes] 0 for the backend default (10 MiB cap).
 * @returns {Promise<string>}
 */
export async function getFullTranscript(resumeId, maxBytes = 0) {
  const api = backend();
  if (!api?.GetTerminalTranscriptFull || !resumeId) return '';
  const text = await api.GetTerminalTranscriptFull(resumeId, maxBytes);
  return typeof text === 'string' ? text : '';
}

/**
 * Read the full transcript content for a resume id with authoritative size /
 * truncation metadata. Use this instead of getFullTranscript when the UI
 * needs to render a truncation banner — comparing string length to file size
 * is wrong for multi-byte UTF-8.
 *
 * @param {string} resumeId
 * @param {number} [maxBytes] 0 for the backend default (10 MiB cap).
 * @returns {Promise<{resumeId: string, text: string, size: number, readBytes: number, truncated: boolean}>}
 */
export async function getTranscriptContent(resumeId, maxBytes = 0) {
  const empty = { resumeId, text: '', size: 0, readBytes: 0, truncated: false };
  const api = backend();
  if (!api?.GetTerminalTranscriptContent || !resumeId) return empty;
  const raw = await api.GetTerminalTranscriptContent(resumeId, maxBytes);
  if (!raw || typeof raw !== 'object') return empty;
  return {
    resumeId: raw.resumeId || resumeId,
    text: typeof raw.text === 'string' ? raw.text : '',
    size: Number.isFinite(raw.size) ? raw.size : 0,
    readBytes: Number.isFinite(raw.readBytes) ? raw.readBytes : 0,
    truncated: Boolean(raw.truncated),
  };
}

/**
 * Read transcript content with known secrets scrubbed (API keys, tokens,
 * private keys replaced with [REDACTED]). Use for copy/export workflows.
 *
 * @param {string} resumeId
 * @param {number} [maxBytes]
 * @returns {Promise<{resumeId: string, text: string, size: number, readBytes: number, truncated: boolean}>}
 */
export async function getTranscriptContentScrubbed(resumeId, maxBytes = 0) {
  const empty = { resumeId, text: '', size: 0, readBytes: 0, truncated: false };
  const api = backend();
  if (!api?.GetTerminalTranscriptContentScrubbed || !resumeId) return empty;
  const raw = await api.GetTerminalTranscriptContentScrubbed(resumeId, maxBytes);
  if (!raw || typeof raw !== 'object') return empty;
  return {
    resumeId: raw.resumeId || resumeId,
    text: typeof raw.text === 'string' ? raw.text : '',
    size: Number.isFinite(raw.size) ? raw.size : 0,
    readBytes: Number.isFinite(raw.readBytes) ? raw.readBytes : 0,
    truncated: Boolean(raw.truncated),
  };
}

/**
 * Persist the terminal label side-car so it survives the session closing.
 * Fire-and-forget by design at the call sites — callers may pass an onError
 * if they want surfacing.
 */
export async function saveTranscriptMetadata({ resumeId, name = '', type = '', sshProfileId = '' }, onError) {
  const api = backend();
  if (!api?.SaveTranscriptMetadata || !resumeId) return;
  try {
    await api.SaveTranscriptMetadata(resumeId, name, type, sshProfileId);
  } catch (error) {
    if (onError) onError(error);
  }
}

/**
 * Backwards-compatible per-data-chunk append. Kept for App.svelte's hot path.
 */
export async function appendTerminalTranscript(resumeId, data, onError) {
  const api = backend();
  if (!api?.AppendTerminalTranscript || !resumeId || !data) return;
  try {
    await api.AppendTerminalTranscript(resumeId, data);
  } catch (error) {
    if (onError) onError(error);
  }
}

/**
 * Delete a single transcript by resume ID.
 * @param {string} resumeId
 * @returns {Promise<{resumeId: string, deleted: boolean, error?: string}>}
 */
export async function deleteTranscript(resumeId) {
  const api = backend();
  if (!api?.DeleteTranscript || !resumeId) return { resumeId, deleted: false, error: 'no backend' };
  return await api.DeleteTranscript(resumeId);
}

/**
 * Delete multiple transcripts. Best-effort: returns per-id results.
 * @param {string[]} resumeIds
 * @returns {Promise<Array<{resumeId: string, deleted: boolean, error?: string}>>}
 */
export async function deleteTranscripts(resumeIds) {
  const api = backend();
  if (!api?.DeleteTranscripts || !resumeIds?.length) return [];
  return await api.DeleteTranscripts(resumeIds);
}

/**
 * Get disk usage stats for stored transcripts.
 * @returns {Promise<{count: number, totalBytes: number}>}
 */
export async function getTranscriptDiskUsage() {
  const api = backend();
  if (!api?.GetTranscriptDiskUsage) return { count: 0, totalBytes: 0 };
  return await api.GetTranscriptDiskUsage();
}

/**
 * Tail excerpt read used by the restored-transcript overlay on restore.
 * @param {string} resumeId
 * @param {number} maxBytes
 * @returns {Promise<string>}
 */
export async function getTranscriptExcerpt(resumeId, maxBytes = 8000) {
  const api = backend();
  if (!api?.GetTerminalTranscriptExcerpt || !resumeId) return '';
  const text = await api.GetTerminalTranscriptExcerpt(resumeId, maxBytes);
  return typeof text === 'string' ? text : '';
}
