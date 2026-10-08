package resources

import (
	"os"
	"strconv"
)

const clkTck = 100 // USER_HZ, what /proc reports regardless of the kernel HZ

func sampleProcesses() ([]Sample, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	pageSize := int64(os.Getpagesize())
	samples := make([]Sample, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue
		}
		data, err := os.ReadFile("/proc/" + e.Name() + "/stat")
		if err != nil {
			continue // exited meanwhile
		}
		if s, ok := parseProcStat(string(data), pageSize, clkTck); ok {
			samples = append(samples, s)
		}
	}
	return samples, nil
}
