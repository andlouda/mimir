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

// Shell-quote for POSIX shells; plain paths stay plain.
export function shellQuotePath(p) {
  if (/^[A-Za-z0-9_./:\\-]+$/.test(p)) return p;
  return `'${p.replace(/'/g, "'\\''")}'`;
}

// C:/x or C:\x → /mnt/c/x for a WSL pane; other paths unchanged.
export function toWslPath(p) {
  const m = /^([A-Za-z]):[\\/](.*)$/.exec(p);
  if (!m) return p;
  return `/mnt/${m[1].toLowerCase()}/${m[2].replace(/\\/g, '/')}`;
}

// Quote a path for the shell behind a pane: cmd.exe knows only double
// quotes, PowerShell takes single quotes with '' inside, POSIX shells the
// usual single-quote form; WSL panes also get the /mnt/<drive> path.
export function quotePathFor(p, terminalType) {
  switch (terminalType) {
    case 'cmd':
      return /[\s&|<>^()]/.test(p) ? `"${p}"` : p;
    case 'powershell':
      return /^[A-Za-z0-9_./:\\-]+$/.test(p) ? p : `'${p.replace(/'/g, "''")}'`;
    case 'wsl':
      return shellQuotePath(toWslPath(p));
    default:
      return shellQuotePath(p);
  }
}

export function droppedPathsText(uriList, terminalType = '') {
  return pathsFromUriList(uriList).map((p) => quotePathFor(p, terminalType)).join(' ');
}
