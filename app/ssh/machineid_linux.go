package ssh

import "os"

// machineIDPaths are the canonical locations of the systemd/D-Bus machine ID.
var machineIDPaths = []string{
	"/etc/machine-id",
	"/var/lib/dbus/machine-id",
}

// platformMachineID reads the Linux machine-id, a stable per-installation
// identifier. It is local to the host and not exposed over the network.
func platformMachineID() string {
	for _, path := range machineIDPaths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if id := string(data); len(id) > 0 {
			return id
		}
	}
	return ""
}
