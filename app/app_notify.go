package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode"

	"mimir/executil"
)

// Desktop notifications for agent events: an agent finished or waits for a
// permission while its pane is not in front. Each platform's own notifier
// is used, always through argv or environment (never a shell string built
// from the text): notify-send on Linux, osascript on macOS, a PowerShell
// WinRT toast on Windows. The text is short and only ever holds the agent
// label, the terminal name and the hook's one-line message.

const (
	notifyMaxLen   = 160
	notifyMinGap   = 5 * time.Second
	notifyTimeout  = 8 * time.Second
	notifyAppTitle = "Mimir"
)

var (
	notifyMu   sync.Mutex
	notifyLast = map[int]time.Time{}
)

// NotifyDesktop shows a notification for a terminal's agent. Bursts per
// terminal are collapsed to one every few seconds; a missing notifier is
// not an error worth surfacing (the in-app badge still shows the state).
func (a *App) NotifyDesktop(terminalID int, title, body string) error {
	notifyMu.Lock()
	if last, ok := notifyLast[terminalID]; ok && time.Since(last) < notifyMinGap {
		notifyMu.Unlock()
		return nil
	}
	notifyLast[terminalID] = time.Now()
	notifyMu.Unlock()

	name, args, env, ok := notifyCommand(runtime.GOOS, cleanNotifyText(title), cleanNotifyText(body))
	if !ok {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), notifyTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	if env != nil {
		cmd.Env = env
	}
	executil.HideConsoleWindow(cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("notification failed: %s", firstOutputLine(strings.TrimSpace(string(out))))
	}
	return nil
}

var ansiSequence = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(\x07|\x1b\\)|\x1b[@-Z\\-_]`)

// cleanNotifyText keeps one printable line of bounded length: escape
// sequences, control characters and line breaks are removed.
func cleanNotifyText(s string) string {
	s = ansiSequence.ReplaceAllString(s, "")
	s = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return ' '
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > notifyMaxLen {
		s = strings.TrimSpace(s[:notifyMaxLen]) + "…"
	}
	return s
}

// windowsToastScript shows a toast through the WinRT API from Windows
// PowerShell; title and body arrive via environment variables so no text
// is ever interpolated into the script. The AUMID is PowerShell's own, the
// one registered app id that shows toasts without an installer.
const windowsToastScript = `$null = [Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime]
$null = [Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime]
$x = [Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent([Windows.UI.Notifications.ToastTemplateType]::ToastText02)
$t = $x.GetElementsByTagName('text')
$null = $t.Item(0).AppendChild($x.CreateTextNode($env:MIMIR_NOTIFY_TITLE))
$null = $t.Item(1).AppendChild($x.CreateTextNode($env:MIMIR_NOTIFY_BODY))
$id = '{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\WindowsPowerShell\v1.0\powershell.exe'
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier($id).Show([Windows.UI.Notifications.ToastNotification]::new($x))`

// notifyCommand builds the platform command. ok is false when the platform
// has no known notifier.
func notifyCommand(goos, title, body string) (name string, args []string, env []string, ok bool) {
	switch goos {
	case "linux":
		return "notify-send", []string{"-a", notifyAppTitle, "-i", "mimir", "--", title, body}, nil, true
	case "darwin":
		return "osascript", []string{
			"-e", "on run argv",
			"-e", "display notification (item 2 of argv) with title (item 1 of argv)",
			"-e", "end run",
			title, body,
		}, nil, true
	case "windows":
		env = append(os.Environ(), "MIMIR_NOTIFY_TITLE="+title, "MIMIR_NOTIFY_BODY="+body)
		return "powershell.exe", []string{"-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-Command", windowsToastScript}, env, true
	}
	return "", nil, nil, false
}
