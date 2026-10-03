package main

import (
	"errors"
	"fmt"
	"html"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

const tokenElevation = 20 // TOKEN_INFORMATION_CLASS TokenElevation

// processElevated reports whether a process runs as administrator. Windows doesn't let a
// normal program move or restyle the windows of such a process.
func processElevated(pid uint32) bool {
	h, err := syscall.OpenProcess(processQueryLimitedInformation, false, pid)
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(h)
	return tokenElevated(h)
}

func isElevated() bool {
	h, err := syscall.GetCurrentProcess()
	return err == nil && tokenElevated(h)
}

func tokenElevated(process syscall.Handle) bool {
	var token syscall.Token
	if syscall.OpenProcessToken(process, syscall.TOKEN_QUERY, &token) != nil {
		return false
	}
	defer token.Close()
	var elevated, n uint32
	if syscall.GetTokenInformation(token, tokenElevation, (*byte)(unsafe.Pointer(&elevated)), 4, &n) != nil {
		return false
	}
	return elevated != 0
}

// relaunchElevated starts FancyBorderless again with administrator rights; Windows shows
// its confirmation prompt. waitForExit makes the new instance wait until this one is gone.
func relaunchElevated(waitForExit bool) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	args := ""
	if waitForExit {
		args = restartFlag
	}
	r, _, _ := procShellExecuteW.Call(0, uintptr(unsafe.Pointer(utf16Ptr("runas"))), uintptr(unsafe.Pointer(utf16Ptr(exe))),
		uintptr(unsafe.Pointer(utf16Ptr(args))), 0, swShowNormal)
	if r <= 32 {
		return errors.New("Windows didn't start it with administrator rights (the prompt may have been declined)")
	}
	return nil
}

// A program in the Run registry key always starts without administrator rights, so in
// administrator mode "Start with Windows" uses a logon task instead, the way PowerToys does.
const taskName = "FancyBorderless"

const taskXML = `<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo><Description>Starts FancyBorderless with administrator rights when you sign in.</Description></RegistrationInfo>
  <Triggers><LogonTrigger><Enabled>true</Enabled><UserId>%[1]s</UserId></LogonTrigger></Triggers>
  <Principals><Principal id="Author"><UserId>%[1]s</UserId><LogonType>InteractiveToken</LogonType><RunLevel>HighestAvailable</RunLevel></Principal></Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>
    <Priority>7</Priority>
  </Settings>
  <Actions Context="Author"><Exec><Command>%[2]s</Command></Exec></Actions>
</Task>`

func logonTaskExists() bool {
	return runHidden("schtasks", "/Query", "/TN", taskName) == nil
}

func createLogonTask() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	u, err := user.Current()
	if err != nil {
		return err
	}
	xml := fmt.Sprintf(taskXML, html.EscapeString(u.Username), html.EscapeString(exe))
	// schtasks reads the task definition as UTF-16.
	data := []uint16{0xFEFF}
	data = append(data, utf16.Encode([]rune(xml))...)
	buf := make([]byte, len(data)*2)
	for i, c := range data {
		buf[2*i], buf[2*i+1] = byte(c), byte(c>>8)
	}
	path := filepath.Join(os.TempDir(), "FancyBorderless-task.xml")
	if err := os.WriteFile(path, buf, 0o600); err != nil {
		return err
	}
	defer os.Remove(path)
	return runHidden("schtasks", "/Create", "/TN", taskName, "/XML", path, "/F")
}

func deleteLogonTask() error {
	if !logonTaskExists() {
		return nil
	}
	return runHidden("schtasks", "/Delete", "/TN", taskName, "/F")
}

// runHidden runs a console tool without flashing a console window.
func runHidden(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w: %s", name, err, out)
	}
	return nil
}
