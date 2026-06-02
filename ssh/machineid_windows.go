package ssh

import "golang.org/x/sys/windows/registry"

// platformMachineID reads the Windows MachineGuid, a per-installation
// identifier created by the OS. It is not exposed over the network and is
// stable across reboots.
func platformMachineID() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Cryptography`, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return ""
	}
	defer k.Close()

	guid, _, err := k.GetStringValue("MachineGuid")
	if err != nil {
		return ""
	}
	return guid
}
