package machineid

import (
	"os"
	"strings"
)

func readMachineID() string {
	// /etc/machine-id is generated at install time and is stable
	data, err := os.ReadFile("/etc/machine-id")
	if err != nil {
		// Fallback for some distros
		data, err = os.ReadFile("/var/lib/dbus/machine-id")
		if err != nil {
			return ""
		}
	}
	return strings.TrimSpace(string(data))
}
