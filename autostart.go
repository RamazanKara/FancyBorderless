package main

import (
	"os"
	"strings"
	"syscall"
	"unsafe"
)

var (
	advapi32            = syscall.NewLazyDLL("advapi32.dll")
	procRegSetValueExW  = advapi32.NewProc("RegSetValueExW")
	procRegDeleteValueW = advapi32.NewProc("RegDeleteValueW")
)

// Start with Windows uses the current user's Run key, the same place Task Manager's Startup
// apps page reads from.
const (
	runKeyPath   = `Software\Microsoft\Windows\CurrentVersion\Run`
	runValueName = "FancyBorderless"
)

func startsWithWindows() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	var key syscall.Handle
	if syscall.RegOpenKeyEx(syscall.HKEY_CURRENT_USER, utf16Ptr(runKeyPath), 0, syscall.KEY_QUERY_VALUE, &key) != nil {
		return false
	}
	defer syscall.RegCloseKey(key)
	buf := make([]uint16, 1024)
	n := uint32(len(buf) * 2)
	if syscall.RegQueryValueEx(key, utf16Ptr(runValueName), nil, nil, (*byte)(unsafe.Pointer(&buf[0])), &n) != nil {
		return false
	}
	return strings.EqualFold(strings.Trim(syscall.UTF16ToString(buf), `"`), exe)
}

func setStartWithWindows(on bool) error {
	var key syscall.Handle
	if err := syscall.RegOpenKeyEx(syscall.HKEY_CURRENT_USER, utf16Ptr(runKeyPath), 0, syscall.KEY_SET_VALUE, &key); err != nil {
		return err
	}
	defer syscall.RegCloseKey(key)
	name := utf16Ptr(runValueName)
	if !on {
		if r, _, _ := procRegDeleteValueW.Call(uintptr(key), uintptr(unsafe.Pointer(name))); r != 0 && r != uintptr(syscall.ERROR_FILE_NOT_FOUND) {
			return syscall.Errno(r)
		}
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	data, err := syscall.UTF16FromString(`"` + exe + `"`)
	if err != nil {
		return err
	}
	if r, _, _ := procRegSetValueExW.Call(uintptr(key), uintptr(unsafe.Pointer(name)), 0, syscall.REG_SZ,
		uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)*2)); r != 0 {
		return syscall.Errno(r)
	}
	return nil
}
