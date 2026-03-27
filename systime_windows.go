//go:build windows

package main

import (
	"fmt"
	"syscall"
	"time"
	"unsafe"
)

var modkernel32 = syscall.NewLazyDLL("kernel32.dll")
var procSetSystemTime = modkernel32.NewProc("SetSystemTime")

type systemtime struct {
	Year         uint16
	Month        uint16
	DayOfWeek    uint16
	Day          uint16
	Hour         uint16
	Minute       uint16
	Second       uint16
	Milliseconds uint16
}

func setSystemTime(t time.Time) error {
	t = t.UTC()
	st := systemtime{
		Year:         uint16(t.Year()),
		Month:        uint16(t.Month()),
		Day:          uint16(t.Day()),
		Hour:         uint16(t.Hour()),
		Minute:       uint16(t.Minute()),
		Second:       uint16(t.Second()),
		Milliseconds: uint16(t.Nanosecond() / 1e6),
	}
	r, _, err := procSetSystemTime.Call(uintptr(unsafe.Pointer(&st)))
	if r == 0 {
		return fmt.Errorf("SetSystemTime: %w", err)
	}
	return nil
}
