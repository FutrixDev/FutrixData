package machineid

import (
	"os/exec"
	"strings"
)

func readMachineID() string {
	// MachineGuid from Windows registry — stable per installation
	out, err := exec.Command("reg", "query",
		`HKLM\SOFTWARE\Microsoft\Cryptography`,
		"/v", "MachineGuid").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "MachineGuid") {
			fields := strings.Fields(trimmed)
			if len(fields) >= 3 {
				return fields[len(fields)-1]
			}
		}
	}
	return ""
}
