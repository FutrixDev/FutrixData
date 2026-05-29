package machineid

import (
	"crypto/sha256"
	"encoding/hex"
)

const salt = "futrixdata-device-v1"

// DeviceID returns a stable device identifier derived from the machine's
// hardware/OS identity. The same physical machine always produces the
// same device ID, even across reinstalls or file deletions.
func DeviceID() string {
	raw := readMachineID()
	if raw == "" {
		return ""
	}
	hash := sha256.Sum256([]byte(salt + raw))
	return "device_" + hex.EncodeToString(hash[:16]) // 32 hex chars
}
