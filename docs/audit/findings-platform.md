# Findings — platform

Scope: the three webviews (WebKitGTK on Linux incl. Wayland, WebView2 on
Windows, WKWebView on macOS) and the platform-specific backend paths:
`app/main.go`, `app/inputmethod_linux.go`, `app/desktop`, `app/terminal/terminal.go`
(ConPTY) vs `terminal_unix.go`, `app/executil`, `app/app_notify.go`,
`app/app_agents.go`, `app/ssh/secrets.go`, `.github/workflows/release.yml`;
frontend CSS features, clipboard, drag & drop, fonts, zoom and platform
detection. Frontend paths are relative to `app/frontend/src`.

Everything here is code reasoning: the Playwright mocks run in Chromium and
cannot stand in for WebKitGTK/WebView2, and no `wails dev` session exists.
Wails internals were read from the module cache
(`github.com/wailsapp/wails/v2@v2.10.1`, cited as `wails/…`). PR #48
(`origin/feat/terminal-rendering`, not merged) is referenced where it changes
the picture; the branch under audit still hard-codes the GPU policy.

---

### [P1] `GTK_IM_MODULE=gtk-im-context-simple` is forced: no CJK / complex-script input on Linux
- Area: `app/inputmethod_linux.go:22-23`
- Platform: linux-webkitgtk
- Symptom: On a GNOME/KDE Wayland or X11 session where the user never exported `GTK_IM_MODULE` (the default on GNOME: ibus and fcitx5 are reached through the compositor's text-input protocol or GTK's module auto-detection, not through the variable), Mimir starts with the "simple" context. ibus/fcitx never activate inside the window, so Japanese, Chinese, Korean, Vietnamese etc. cannot be typed anywhere in Mimir — not in terminals, not in the SSH password field or the search box. Conversely, users who *did* export `GTK_IM_MODULE=ibus|fcitx` (common via im-config / fcitx5 profile.d) keep the original dropped/doubled-umlaut bug, because the override is skipped for them.
- Repro: code reasoning, confidence medium (mechanism high; share of affected users depends on distro defaults). `LANG=ja_JP.UTF-8` GNOME Wayland with ibus-anthy, no `GTK_IM_MODULE` in the environment → start Mimir → Super+Space switches the indicator but no composition window appears in the webview.
- Cause: `configureInputMethod()` sets the variable before GTK initialises whenever it is absent (`inputmethod_linux.go:22-23`), which on GTK3 pins the `simple` IM context and bypasses `wayland`/`ibus`/`fcitx`. The comment at `:18-19` assumes CJK users always export the variable, which is not the case on GNOME. There is no setting and no CLI flag to turn the workaround off.
- Fix sketch: Make the workaround opt-in or at least opt-out (Settings → "Dead-key workaround", default on only for non-CJK locales derived from `LANG`/`LC_CTYPE`); alternatively fix the double-emit in xterm's composition path (ignore `input` events while `isComposing`) instead of changing the IM module.
- Evidence: `app/inputmethod_linux.go:5-25`, `app/main.go:34-36`; prior art `docs/audit/01-prior-art.md` check 9.

### [P1] Dropping a file onto the window navigates the webview away and destroys every terminal
- Area: `App.svelte` (no window-level `dragover`/`drop` handler) with `app/main.go:52-83` (no `DragAndDrop` option)
- Platform: windows-webview2, linux-webkitgtk
- Symptom: A user drags a file from Explorer/Nautilus onto a terminal pane (the usual intent in a terminal: paste the path). WebView2 and WebKitGTK treat an uncancelled drop as "load this URL": the page navigates to `file:///…`, the whole Svelte app is unmounted, all xterm instances and in-memory state are gone, and there is no back button (no browser chrome). Only a restart recovers.
- Repro: code reasoning, confidence medium (high for WebView2/Chromium behaviour; WebKitGTK's default drag-destination mask includes `Load`; WKWebView on macOS excludes `Load` by default and is likely safe).
- Cause: `dragover`/`drop` are only cancelled on the Sidebar folder rows (`Sidebar.svelte:213-215, 249-251`) and on the split drop overlays while a pane drag is active (`SplitPane.svelte:428-430`); nothing cancels the default for file drops elsewhere. Wails keeps the webview's native drop target (`wails/internal/frontend/desktop/linux/window.c:508-516`: `gtk_drag_dest_unset` only when `disableWebViewDragAndDrop`), and `options.DragAndDrop{DisableWebViewDrop: true}` (`wails/pkg/options/options.go:195-203`) is not set in `main.go`.
- Fix sketch: Add a window-level `dragover`/`drop` listener that calls `preventDefault()` for `dataTransfer.types` containing `Files`/`text/uri-list` and, as a feature, pastes the shell-quoted path into the focused terminal; set `DragAndDrop: &options.DragAndDrop{EnableFileDrop: true}` to receive paths through Wails on Linux/Windows.
- Evidence: `app/main.go:52-83`, `lib/Sidebar.svelte:213-251`, `lib/SplitPane.svelte:296, 428-430`, `wails/.../linux/window.c:498-516`, `wails/pkg/options/options.go:195-203`.

### [P1] Linux release is linked against webkit2gtk-4.0, which current distributions no longer ship
- Area: `.github/workflows/release.yml:74` (with `:35`)
- Platform: linux-webkitgtk
- Symptom: The published `mimir-linux-amd64` binary fails at launch with `error while loading shared libraries: libwebkit2gtk-4.0.so.37` on Fedora 40+, Debian 13, Ubuntu 25.04+ and current Arch — all of which only ship the `4.1` (libsoup3) or `6.0` ABI. The README's install line (`README.md:155`) also names `libwebkit2gtk-4.0-dev`, so users following it on those distros hit a missing package.
- Repro: code reasoning, confidence medium (the build matrix is fixed to `ubuntu-22.04` with the 4.0 dev package; which distros still carry 4.0 compat packages varies).
- Cause: `release.yml:35` builds on `ubuntu-22.04` and `:74` installs `libwebkit2gtk-4.0-dev`; the Wails build does not pass `-tags webkit2_41`, so the binary dlopens the 4.0 soname. Wails v2 supports the 4.1 ABI behind that build tag.
- Fix sketch: Build with `-tags webkit2_41` (and `libwebkit2gtk-4.1-dev`) as the default Linux artifact, optionally publish both; document the runtime dependency (`libwebkit2gtk-4.1-0`) in the README and in the update dialog's error text.
- Evidence: `.github/workflows/release.yml:35, 74`, `README.md:155`.

### [P2] UI and terminal fonts are fetched from Google Fonts at every start; cells are measured before the font arrives
- Area: `app/frontend/index.html:10` (with `:6` CSP and `lib/actions/terminalActions.js:116`)
- Platform: all
- Symptom: (a) Offline, behind a corporate proxy or on a Windows VDI without internet, DM Sans and JetBrains Mono never load: the UI renders in Arial (Windows), DejaVu Sans (Linux) or Helvetica (macOS), and the terminal in Fira Code / Cascadia Code / the engine's `monospace` default (Courier New on WebView2, DejaVu Sans Mono on WebKitGTK) — three visibly different apps. (b) Online, `display=swap` swaps JetBrains Mono in *after* the first terminals have measured their cell size with the fallback font; xterm 6 does not re-measure on font load, so glyphs overlap or leave gaps until the next resize. (c) A security-conscious operations tool contacts `fonts.googleapis.com`/`fonts.gstatic.com` on every launch; the CSP at `index.html:6` is widened just for that.
- Repro: code reasoning, confidence high (the swap race is xterm's documented behaviour; the font list and the lack of `document.fonts.ready` are on this branch).
- Cause: `<link href="https://fonts.googleapis.com/css2?family=JetBrains+Mono…&family=DM+Sans…&display=swap">` at `index.html:10`; `terminalActions.js:116` lists `'JetBrains Mono'` first; no `document.fonts.ready`/`FontFaceSet` hook exists in `terminalActions.js` or `terminals/xtermLifecycle.js`. PR #48 bundles JetBrains Mono via `@fontsource` and re-measures (`main.js` diff, `xtermLifecycle.js` diff) but leaves the Google link and the remote DM Sans in `index.html` (not in the PR file list).
- Fix sketch: Bundle both fonts (woff2 via `@fontsource`, as PR #48 does for the mono face), drop the Google `<link>`, `preconnect` and the two CSP origins; create terminals after `document.fonts.ready` or call `fit()`/`clearTextureAtlas` on `document.fonts.onloadingdone`.
- Evidence: `app/frontend/index.html:6-10`, `lib/actions/terminalActions.js:116`, `styles/base.css:31-32`, `style.css:13`, PR #48 diff of `main.js`.

### [P2] PowerShell/cmd prompt override discards the user's prompt and silently kills Mimir's own cwd/history beacon
- Area: `lib/actions/terminalActions.js:311` (with `:314` and `app/terminal/terminal.go:51-80`)
- Platform: windows-webview2
- Symptom: Every new PowerShell terminal shows a blank screen, then `user ❯`; oh-my-posh/Starship/posh-git prompts from `$PROFILE` are gone. With history enabled, the backend had just installed `function global:prompt` that emits OSC 7337 (`terminal.go:52-79`); the frontend's `function prompt { … }; cls` typed one second later replaces it, so cwd tracking, the `terminal-prompt-<id>` beacon, "open file browser here", agent cwd detection and the history dashboard stop working for PowerShell. The typed setup line also lands in PSReadLine history (first Up-arrow recalls it) and in `Get-History`. cmd gets `prompt %USERNAME% $G$S& cls`, dropping a custom `PROMPT` variable. Both `cls` the screen, erasing any banner or error from the profile.
- Repro: code reasoning, confidence high. 1. Enable history (`~/.config/mimir/history_enabled`). 2. New PowerShell terminal. 3. `cd C:\Temp` → no `terminal-prompt-<id>` event (the prompt function no longer writes the OSC).
- Cause: `terminalActions.js:306-318` writes the prompt redefinition for every non-tmux, non-ssh type; the backend's profile is loaded via `powershell.exe -NoLogo -NoExit -ExecutionPolicy Bypass -File …` (`terminal.go:135`) *before* that line arrives, so the later definition wins.
- Fix sketch: Remove the frontend prompt/cls writes; put the beacon into the profile as a wrapper that calls the previous `prompt` (save `$function:prompt` first, same for cmd via `PROMPT=$P$G` only when unset); never type setup commands through the PTY.
- Evidence: `lib/actions/terminalActions.js:306-318`, `app/terminal/terminal.go:51-80, 126-136`; prior art check 15.

### [P2] bash/zsh rc wrapper overwrites `PS1`/`PROMPT` after sourcing the user's rc file
- Area: `app/terminal/terminal_unix.go:293` (with `:306`)
- Platform: all (local bash/zsh on Linux and macOS, WSL bash via `terminal.go:27`)
- Symptom: Starship, oh-my-posh, bash-git-prompt and hand-written `PS1` are replaced by `\W $` (bash) or `%1~ %#` (zsh) in every Mimir terminal, while the same shell outside Mimir shows the user's prompt. Users read this as "Mimir broke my shell"; zsh frameworks that set `PROMPT` in `precmd` (p10k) win back the prompt on the second line, so the first prompt differs from the rest.
- Repro: code reasoning, confidence high. `eval "$(starship init bash)"` in `~/.bashrc` → new bash terminal in Mimir → plain `dir $` prompt.
- Cause: `localShellLaunch` writes `source ~/.bashrc …\nPS1='\W \$ '` (`terminal_unix.go:293`) and `source ~/.zshrc …\nPROMPT='%1~ %# '` (`:306`), the assignment coming *after* the user's rc; Windows WSL uses the identical `conptyBashRcBase` (`terminal.go:27`). There is no opt-out; `rcMode` exists only for SSH profiles (`app_ssh.go:83`).
- Fix sketch: Only set `PS1`/`PROMPT` when the user's rc left it at bash's/zsh's default (compare against `\s-\v\$ ` / `%m%# `), or drop the override and keep only the beacon hook appended to `PROMPT_COMMAND`/`precmd`.
- Evidence: `app/terminal/terminal_unix.go:285-316`, `app/terminal/terminal.go:27`.

### [P2] WebView2 keeps browser zoom enabled because no `windows.Options` are passed: Ctrl+wheel/pinch outside a pane zooms the whole UI with no way back
- Area: `app/main.go:52-83` (no `Windows:` options) with `lib/SplitPane.svelte:83-86`
- Platform: windows-webview2
- Symptom: Ctrl+wheel over the sidebar, Settings or the agent panel — or a touchpad pinch anywhere — zooms the entire Mimir page (Chromium page zoom). Mimir's Ctrl+0 only resets the *terminal* font size (`keyboardShortcuts.js:137`), and Wails disables the browser accelerator keys (`wails/internal/frontend/desktop/windows/frontend.go:575`), so the browser's own Ctrl+0 does nothing either. The UI stays zoomed until restart; over a terminal pane both zooms (Mimir's and the page's) can apply when the wheel reaches a non-terminal ancestor.
- Repro: code reasoning, confidence medium-high. Wails only calls `PutIsZoomControlEnabled(opts.IsZoomControlEnabled)` / `PutIsPinchZoomEnabled` inside `if opts := f.frontendOptions.Windows; opts != nil` (`frontend.go:557-566`); with `Windows == nil` the WebView2 defaults (both enabled) remain.
- Cause: `main.go` sets `Linux` options but no `Windows:` block; `handleTerminalWheel` cancels Ctrl+wheel only inside a pane (`SplitPane.svelte:79-86`).
- Fix sketch: Add `Windows: &windows.Options{IsZoomControlEnabled: false, DisablePinchZoom: true, Theme: windows.Dark}` and a window-level `wheel` listener that cancels `ctrlKey` wheels so Mimir's zoom is the only one on every platform.
- Evidence: `app/main.go:65-69`, `lib/SplitPane.svelte:79-92, 419`, `lib/actions/keyboardShortcuts.js:24-28, 137`, `wails/.../windows/frontend.go:557-575`.

### [P2] `navigator.clipboard.writeText` is used on three copy buttons; on `wails://` (Linux, macOS) the API is absent
- Area: `lib/ActivityLogViewer.svelte:82` (also `lib/modals/TranscriptViewerModal.svelte:343, 354`)
- Platform: linux-webkitgtk, macos-webkit
- Symptom: "Copy JSON" in the activity log and "Copy all"/"Copy scrubbed" in the transcript viewer fail with `Copy failed: undefined is not an object (evaluating 'navigator.clipboard.writeText')`. Every other copy path in the app works because it uses the Wails runtime.
- Repro: code reasoning, confidence medium. `navigator.clipboard` exists only in secure contexts; Wails serves the page from `wails://wails/` on Linux and macOS (`wails/.../linux/frontend.go:105`) and registers the scheme without `webkit_security_manager_register_uri_scheme_as_secure` (`window.c:505`), so WebKit does not treat it as potentially trustworthy. Windows is fine (`http://wails.localhost/`, `frontend.go:37`, loopback = secure).
- Cause: Direct `navigator.clipboard` calls at `ActivityLogViewer.svelte:82`, `TranscriptViewerModal.svelte:343, 354` instead of `ClipboardSetText` from `wailsjs/runtime` used in `SplitPane.svelte:178`, `AgentTranscriptPanel.svelte:311`, `DotEnvViewerModal.svelte:95`.
- Fix sketch: Replace the three calls with `ClipboardSetText`; add a lint rule or wrapper (`lib/clipboard.js`) so `navigator.clipboard` is never used directly.
- Evidence: `lib/ActivityLogViewer.svelte:78-87`, `lib/modals/TranscriptViewerModal.svelte:340-358`, `wails/internal/frontend/desktop/linux/frontend.go:105`, `wails/.../linux/window.c:505`, `wails/.../windows/frontend.go:37, 90`.

### [P2] PR #48 preview: the Wayland fix sets `WEBKIT_DISABLE_DMABUF_RENDERER`, which WebKitGTK ≥ 2.46 ignores
- Area: `app/app_gpu.go:169-174` on `origin/feat/terminal-rendering` (with `app/main.go:38-42` in that branch)
- Platform: linux-webkitgtk (Wayland)
- Symptom: The new Settings card promises that "auto"/"on" switches WebKit to its older renderer to remove artifacts, stale regions and tearing. On Fedora 41+, Ubuntu 24.10+, Arch and any distro with WebKitGTK 2.46 or newer the DMA-BUF renderer is the only renderer left and the variable has no effect, so users toggle the setting, restart as instructed and see no change — with no hint why. The variable that still works on 2.46+ is `WEBKIT_DISABLE_COMPOSITING_MODE=1` (software compositing).
- Repro: code reasoning, confidence medium (WebKitGTK 2.46 release notes removed the non-DMA-BUF path; exact behaviour of the variable on 2.46.x not re-verified here).
- Cause: `applyWaylandFix()` only knows the one variable (`app_gpu.go:169-174`); the decision table `shouldDisableDMABUF` (`:154-166`) does not consult the WebKitGTK version (`webkit_get_major_version()` is available through cgo, or `pkg-config --modversion` at build time). Also relevant to the renderer: the hard-coded `WebviewGpuPolicyNever` on this branch (`app/main.go:68`) maps to `WEBKIT_HARDWARE_ACCELERATION_POLICY_NEVER` (`wails/.../linux/window.c:534-540`), so no WebGL context exists and PR #48's WebGL addon falls back to DOM unless the user also picks "on demand"; the PR does not make that coupling explicit in the UI copy.
- Fix sketch: Detect the runtime WebKitGTK version and pick the variable (`WEBKIT_DISABLE_DMABUF_RENDERER` < 2.46, `WEBKIT_DISABLE_COMPOSITING_MODE` ≥ 2.46); show the detected version and the effective variable in the Settings card; in the renderer card, say that WebGL needs GPU policy ≠ never.
- Evidence: `origin/feat/terminal-rendering:app/app_gpu.go:77-174`, `git diff main origin/feat/terminal-rendering -- app/main.go` (lines 38-42, 71-74, 95-107), `app/main.go:65-69` (this branch), `wails/.../linux/window.c:530-541`.

### [P2] Window `BackgroundColour` (#1b2636) does not match the page background (#0c0e14)
- Area: `app/main.go:59`
- Platform: all (most visible on linux-webkitgtk and windows-webview2)
- Symptom: A blue-grey frame flashes at launch before the CSS paints, and the same colour shows in freshly exposed regions during window resizes/maximise and while WebKitGTK has not repainted (e.g. after wake-up). On Wayland the GTK CSD shadow/edges briefly show the wrong colour too.
- Repro: code reasoning, confidence high. Wails applies the colour to both the webview and the window (`wails/internal/frontend/desktop/linux/window.c:148-170`), WebView2 uses it as the default background colour.
- Cause: `BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1}` vs `--bg-void: #0c0e14` (`styles/base.css:10`) and `XTERM_THEME.background '#0c0e14'` (`terminalActions.js:27`).
- Fix sketch: Set `BackgroundColour` to `{R: 12, G: 14, B: 20, A: 255}` and keep it in one place (a Go constant mirrored in a CSS token comment).
- Evidence: `app/main.go:59`, `styles/base.css:10`, `lib/actions/terminalActions.js:26-27`, `wails/.../linux/window.c:148-170`; prior art check 11.

### [P3] System keyring is probed with a write inside `NewApp`, before any window exists
- Area: `app/ssh/secrets.go:150` (called from `app/app.go:99` inside `NewApp`, `main.go:39`)
- Platform: linux-webkitgtk (also macos-webkit on first run)
- Symptom: On Linux with a locked GNOME keyring (autologin, or a keyring password different from the login password) the first thing the user sees is a system dialog "An application wants access to the keyring 'Login', but it is locked" with no Mimir window or icon behind it; cancelling it silently drops Mimir to the encrypted-file backend for this run and the master-password gate appears instead. On macOS the probe writes a `mimir` item to the login keychain on every start.
- Repro: code reasoning, confidence medium.
- Cause: `NewSecretStore` writes and deletes a probe item (`secrets.go:148-153`) to decide the backend; it runs from `NewApp` (`app.go:99`) which `main.go:39` calls before `wails.Run`.
- Fix sketch: Probe lazily (first secret read/write, or in `OnStartup` after the window is mapped) and remember the result; on failure show a Mimir-branded explanation instead of switching backends silently.
- Evidence: `app/ssh/secrets.go:136-160`, `app/app.go:68, 99`, `app/main.go:39, 52`.

### [P3] Window chrome follows the OS light theme while the app is always dark
- Area: `app/main.go:52-83` (no `Windows.Theme`, no `Mac.Appearance`, no GTK dark hint)
- Platform: all
- Symptom: Windows 10/11 in light mode: white title bar and light min/max/close buttons over a near-black app. macOS light appearance: light title bar. GNOME/KDE light theme: light CSD/SSD decorations and light GTK scrollbars in the WebKit popups. The contrast seam is the first thing a new user sees.
- Repro: code reasoning, confidence high.
- Cause: `main.go` passes neither `Windows: &windows.Options{Theme: windows.Dark}` nor `Mac: &mac.Options{Appearance: mac.NSAppearanceNameDarkAqua}`; Wails Linux does not set `gtk-application-prefer-dark-theme` (`wails/.../linux/window.c` has no `prefer-dark`).
- Fix sketch: Set the two options; on Linux set `gtk-application-prefer-dark-theme` on `GtkSettings` before `wails.Run` (cgo) or document the limitation.
- Evidence: `app/main.go:52-83`, `wails/.../linux/window.c` (no `prefer-dark` match).

### [P3] Desktop notifications are attributed to helper programs, not to Mimir
- Area: `app/app_notify.go:103-117`
- Platform: all
- Symptom: Linux: `notify-send -a Mimir -i mimir` without `-h string:desktop-entry:mimir` — GNOME 45+ lists the notification under "notify-send"/unknown app, per-app notification settings and Do-Not-Disturb exceptions cannot target Mimir, and clicking does nothing. macOS: `osascript … display notification` shows as **Script Editor**; on macOS 13+ the user must grant Script Editor notification permission, and clicking opens Script Editor. Windows: the toast uses PowerShell's AUMID (`app_notify.go:98`), so it is titled "Windows PowerShell", clicking launches a PowerShell console, and each notification spawns a `powershell.exe` (~0.5 s, CPU spike).
- Repro: code reasoning, confidence high.
- Cause: `notifyCommand` chooses the three shell-outs at `app_notify.go:103-117`; no desktop-entry hint, no AUMID registration, no `UNUserNotificationCenter`.
- Fix sketch: Linux: add `-h string:desktop-entry:mimir` (entry exists via `desktop.Install`). Windows: register an AUMID with a shortcut once (or use `go-toast`), reuse one hidden PowerShell or a native WinRT call. macOS: `terminal-notifier`-style bundle or a cgo `UNUserNotificationCenter` call; at minimum document the Script Editor permission.
- Evidence: `app/app_notify.go:88-117`, `app/desktop/desktop_linux.go:14-24`.

### [P3] Paste is left entirely to the browser default; Ctrl+Shift+V is not handled explicitly
- Area: `lib/actions/terminalActions.js:136-152` (copy is handled, paste is not)
- Platform: linux-webkitgtk (primarily)
- Symptom: Ctrl+Shift+C is handled explicitly so it works on every engine, but the matching terminal paste chord Ctrl+Shift+V relies on the webview firing a `paste` event for it. Chromium/WebView2 does (paste as plain text); WebKitGTK's key-binding table maps Ctrl+V and Shift+Insert to Paste, and whether Ctrl+Shift+V reaches `paste` depends on the WebKitGTK version. Plain Ctrl+V is forwarded by xterm as `\x16` (quoted-insert in bash, image paste in Claude Code), so when the chord is not bound the only reliable paste is the context menu.
- Repro: code reasoning, confidence low (engine binding table not verified for 2.44/2.46).
- Cause: `attachCustomKeyEventHandler` (`terminalActions.js:136-152`) only intercepts copy; the `paste` listener (`:347-357`) is passive. The context-menu path already uses `ClipboardGetText` (`SplitPane.svelte:251-260`).
- Fix sketch: Handle Ctrl+Shift+V and Shift+Insert in the custom key handler via `ClipboardGetText()` → `terminal.paste()` (bracketed paste preserved), mirroring the copy branch; keep the browser `paste` event as fallback.
- Evidence: `lib/actions/terminalActions.js:136-152, 347-357`, `lib/SplitPane.svelte:251-260`.

### [P3] No `forced-colors`/high-contrast handling: Windows High Contrast overrides terminal and badge colours
- Area: `style.css` / `styles/base.css` (no `@media (forced-colors: active)`, no `forced-color-adjust`)
- Platform: windows-webview2
- Symptom: With a Windows High Contrast theme active, Chromium's forced-colors mode replaces `color`/`background-color` on every element. xterm 6's DOM renderer paints cells with inline colours, so all ANSI colours collapse to the system text colour, the selection and cursor become invisible, agent badges and state colours in the sidebar lose meaning; focus rings are replaced by system ones.
- Repro: code reasoning, confidence medium.
- Cause: grep over `src/**/*.{svelte,css}` finds no `forced-colors`, `prefers-contrast` or `forced-color-adjust`.
- Fix sketch: Add `.xterm { forced-color-adjust: none; }` and a small `@media (forced-colors: active)` block that keeps badge/state meaning via borders/icons; test with `emulateMedia({ forcedColors: 'active' })` in Playwright.
- Evidence: CSS feature grep (`:has`, `@container`, `backdrop-filter`, … counts) — `forced-colors` 0 hits; `lib/actions/terminalActions.js:26-27` (xterm theme).

### [P3] No minimum window size
- Area: `app/main.go:52-55`
- Platform: all
- Symptom: The window can be shrunk to the WM minimum (a few hundred pixels or less on Wayland/Windows); the sidebar (≈220 px) plus a terminal cannot fit, xterm `fit()` yields 0–2 columns and the `@container (max-width: 300px)` pane rules (`SplitPane.svelte:776`) are the only degradation path. On tiling Wayland compositors (Sway/Hyprland) a new tile can easily be narrower than that.
- Repro: code reasoning, confidence high (`MinWidth`/`MinHeight` absent; Wails default 0, `wails/.../linux/window.c:231-245` applies whatever is set).
- Cause: `options.App` only sets `Width`/`Height` (`main.go:54-55`).
- Fix sketch: Set `MinWidth: 720, MinHeight: 480` (or whatever the layout domain fixes as the smallest working layout) in `options.App`.
- Evidence: `app/main.go:52-55`, `lib/SplitPane.svelte:776`, `wails/.../linux/window.c:231-245`.

---

## Not found / verified OK

- CSS features: `::-webkit-scrollbar` (`style.css:23-38, 60`) is paired with `scrollbar-width: none` (`:57`) — works on all three engines. `@container`/`container-type` (`style.css:119`, `SplitPane.svelte:776`) supported by WebKitGTK ≥ 2.40, WebView2, Safari 16+. `backdrop-filter` (`SecretUnlockGate.svelte:193`, `FileBrowser.svelte:458`) gets the `-webkit-` prefix in the production CSS (`dist/assets/index-*.css` contains `webkit-backdrop-filter`). No `:has()`, `overflow: overlay`, native CSS nesting, `light-dark()`, `dvh/svh` or `@layer` in use.
- OSC 52 provider is write-only (`terminals/osc52Clipboard.js:9-16`); paste through the browser event and through the context menu both go via `terminal.paste()` so bracketed paste applies (`terminalActions.js:347-357`, `SplitPane.svelte:251-260`). Copy chords Ctrl+Shift+C / Ctrl+Insert handled explicitly (`terminalActions.js:136-148`).
- `Ctrl+Shift+U` is correctly excluded on Linux (ibus Unicode input chord) (`keyboardShortcuts.js:158-160`).
- Platform detection uses `navigator.userAgent.includes('Windows')` (`App.svelte:43`); WebView2's UA contains "Windows NT", and Wails only appends application details (`window.c:529`), so this is reliable. No `window.runtime.Environment()` calls exist — nothing to go wrong there.
- `localStorage` holds a single key (`mimir-notes-width`); all other state is persisted by the backend, so origin/profile differences between `wails://` and `http://wails.localhost` cannot lose user data.
- Desktop entry and icon are installed *before* the window is created (`main.go:41-49`, `desktop_linux.go:26-108`) with `StartupWMClass=mimir` matching `ProgramName` — the Wayland icon association works; `notify-send -i mimir` resolves through the same hicolor icon.
- Windows helper processes are started with `CREATE_NO_WINDOW` (`executil/hide_windows.go`) for notifications and agent process listing; the update helper (`app_update.go:172`) uses `-WindowStyle Hidden` without `executil.HideConsoleWindow`, which may flash a console for one frame during an update — not verified, not counted.
- WSL paths use `\\wsl$\<distro>` (`app_agents.go:750`), still valid on Windows 10 and 11 (`\\wsl.localhost` is an alias).
- Wails disables browser accelerator keys on WebView2 (`frontend.go:575`): Ctrl+F, Ctrl+P, F5 do not trigger find/print/reload on Windows. (The default context menu's "Reload" is still enabled by `EnableDefaultContextMenu: true`, `main.go:82` — covered by the layout/side-panels domain.)
- `TERM=xterm-256color` is set for local PTYs on Unix (`terminal_unix.go:440`); ConPTY sessions inherit Mimir's environment, which is the right behaviour for PowerShell/cmd (`TERM` is irrelevant there; WSL sets its own).
- Fractional scaling on Wayland: nothing in the app assumes integer `devicePixelRatio` (no `devicePixelRatio`/`matchMedia` use in `src/`); DPR change handling is the theme-dpi domain's topic.
