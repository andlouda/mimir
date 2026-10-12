// Which local shells the "+ New" picker offers, and the remembered default.
export const isWindowsPlatform = typeof navigator !== 'undefined' && navigator.userAgent.includes('Windows');
export const DEFAULT_TERMINAL_TYPE_KEY = 'mimir-default-terminal-type';

// The static list shown before the backend reports what is installed.
export function defaultTerminalTypeOptions() {
  return isWindowsPlatform
    ? [
        { value: 'cmd', label: 'CMD' },
        { value: 'powershell', label: 'PowerShell' },
        { value: 'wsl', label: 'WSL' },
        { value: 'bash', label: 'Bash' },
        { value: 'zsh', label: 'Zsh' },
        { value: 'ssh', label: 'SSH' }
      ]
    : [
        { value: 'bash', label: 'Bash' },
        { value: 'zsh', label: 'Zsh' },
        { value: 'ssh', label: 'SSH' }
      ];
}

export function preferredTerminalType(options) {
  const order = isWindowsPlatform ? ['wsl', 'bash', 'powershell', 'cmd'] : ['bash', 'zsh'];
  return order.map((value) => options.find((option) => option.value === value)).find(Boolean) || options[0];
}

export function readDefaultTerminalType(options) {
  try {
    const saved = localStorage.getItem(DEFAULT_TERMINAL_TYPE_KEY);
    if (saved) return saved;
  } catch (error) {
    console.error('Failed to read default terminal type:', error);
  }
  return preferredTerminalType(options)?.value || (isWindowsPlatform ? 'wsl' : 'bash');
}
