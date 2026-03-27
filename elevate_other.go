//go:build !windows

package main

// On non-Windows platforms the service installer (systemd/launchd) handles
// privilege escalation; no in-process elevation is needed.
func needsElevation() bool      { return false }
func relaunchAsAdmin(_ string) error { return nil }
