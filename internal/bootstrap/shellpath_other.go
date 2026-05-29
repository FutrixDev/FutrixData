//go:build !darwin && !linux

package bootstrap

// EnrichPath is a no-op on Windows where GUI apps share the same
// system/user PATH as terminal sessions.
func EnrichPath() {}
