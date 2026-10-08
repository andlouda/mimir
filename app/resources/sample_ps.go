//go:build !linux && !windows

package resources

import (
	"context"
	"os/exec"
	"time"
)

func sampleProcesses() ([]Sample, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ps", "-axo", "pid=,ppid=,rss=,time=").Output()
	if err != nil && len(out) == 0 {
		return nil, err
	}
	return parsePS(string(out)), nil
}
