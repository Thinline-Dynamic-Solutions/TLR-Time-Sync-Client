//go:build windows

package main

import (
	"fmt"
	"time"

	"golang.org/x/sys/windows"
)

func setSystemTime(t time.Time) error {
	st := windows.Systemtime{
		Year:         uint16(t.Year()),
		Month:        uint16(t.Month()),
		Day:          uint16(t.Day()),
		Hour:         uint16(t.Hour()),
		Minute:       uint16(t.Minute()),
		Second:       uint16(t.Second()),
		Milliseconds: uint16(t.Nanosecond() / 1e6),
	}
	if err := windows.SetSystemTime(&st); err != nil {
		return fmt.Errorf("SetSystemTime: %w", err)
	}
	return nil
}
