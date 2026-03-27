//go:build linux || darwin

package main

import (
	"fmt"
	"time"

	"golang.org/x/sys/unix"
)

func setSystemTime(t time.Time) error {
	tv := unix.NsecToTimeval(t.UnixNano())
	if err := unix.Settimeofday(&tv); err != nil {
		return fmt.Errorf("settimeofday: %w", err)
	}
	return nil
}
