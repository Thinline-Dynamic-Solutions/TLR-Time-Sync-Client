//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

func needsElevation() bool {
	return !windows.GetCurrentProcessToken().IsElevated()
}

// relaunchAsAdmin re-launches the current exe with the "runas" ShellExecute
// verb, which triggers the Windows UAC prompt, then passes "install -config
// <absPath>" as arguments so the elevated process auto-installs the service.
func relaunchAsAdmin(configPath string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot determine executable path: %w", err)
	}

	absConfig, err := filepath.Abs(configPath)
	if err != nil {
		absConfig = configPath
	}

	args := fmt.Sprintf(`install -config "%s"`, absConfig)

	verbPtr, _ := syscall.UTF16PtrFromString("runas")
	exePtr, _ := syscall.UTF16PtrFromString(exe)
	argsPtr, _ := syscall.UTF16PtrFromString(args)

	shell32 := syscall.NewLazyDLL("shell32.dll")
	shellExecuteW := shell32.NewProc("ShellExecuteW")

	ret, _, _ := shellExecuteW.Call(
		0,
		uintptr(unsafe.Pointer(verbPtr)),
		uintptr(unsafe.Pointer(exePtr)),
		uintptr(unsafe.Pointer(argsPtr)),
		0,
		1, // SW_SHOWNORMAL
	)
	if ret <= 32 {
		return fmt.Errorf("ShellExecuteW returned %d (UAC denied or error)", ret)
	}
	return nil
}
