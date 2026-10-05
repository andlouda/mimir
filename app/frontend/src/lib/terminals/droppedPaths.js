// File drops onto the window: the webview would navigate to the dropped
// file and unmount the whole app (every terminal gone). The window-level
// guard cancels that; when the drop carries file URIs (WebKitGTK, WebView2
// text/uri-list) the paths are pasted into the active terminal, quoted for
// a POSIX shell, without a newline so nothing runs by itself.

export function isFileDrop(dataTransfer) {
  const types = Array.from(dataTransfer?.types || []);
  return types.includes('Files') || types.includes('text/uri-list');
}

export function pathsFromUriList(uriList) {
  return String(uriList || '')
    .split(/\r?\n/)
    .map((l) => l.trim())
    .filter((l) => l && !l.startsWith('#'))
    .map(uriToPath)
    .filter(Boolean);
}

function uriToPath(uri) {
  if (!/^file:\/\//i.test(uri)) return null;
  try {
    const u = new URL(uri);
    let p = decodeURIComponent(u.pathname);
    // file:///C:/x on Windows → C:/x
    if (/^\/[A-Za-z]:\//.test(p)) p = p.slice(1);
    if (u.host && u.host !== 'localhost') p = `//${u.host}${p}`; // UNC
    return p;
  } catch {
    return null;
  }
}

// Shell-quote for POSIX shells (and good enough for PowerShell paths with
// spaces); plain paths stay plain.
export function shellQuotePath(p) {
  if (/^[A-Za-z0-9_./:\\-]+$/.test(p)) return p;
  return `'${p.replace(/'/g, "'\\''")}'`;
}

export function droppedPathsText(uriList) {
  return pathsFromUriList(uriList).map(shellQuotePath).join(' ');
}
