package ssh

import (
	"os/exec"
	"regexp"
)

var ioPlatformUUIDPattern = regexp.MustCompile(`"IOPlatformUUID"\s*=\s*"([^"]+)"`)

// platformMachineID reads the macOS IOPlatformUUID via ioreg, a stable
// hardware-scoped identifier that is local to the machine.
func platformMachineID() string {
	out, err := exec.Command("ioreg", "-rd1", "-c", "IOPlatformExpertDevice").Output()
	if err != nil {
		return ""
	}
	match := ioPlatformUUIDPattern.FindSubmatch(out)
	if len(match) < 2 {
		return ""
	}
	return string(match[1])
}
