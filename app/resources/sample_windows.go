package resources

import (
	"context"
	"os/exec"
	"time"

	"mimir/executil"
)

func sampleProcesses() ([]Sample, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command",
		`Get-CimInstance Win32_Process | ForEach-Object { "$($_.ProcessId)" + [char]9 + "$($_.ParentProcessId)" + [char]9 + "$($_.WorkingSetSize)" + [char]9 + "$($_.KernelModeTime + $_.UserModeTime)" }`)
	executil.HideConsoleWindow(cmd)
	out, err := cmd.Output()
	if err != nil && len(out) == 0 {
		return nil, err
	}
	return parseTSV(string(out)), nil
}
