// FancyBorderless removes the title bar and border from windows FancyZones has snapped, so
// they fill their zone exactly. It works from outside: it reads the property FancyZones puts
// on snapped windows and FancyZones' layout files, and changes window styles and positions
// through the regular Windows API. Nothing is loaded into other processes.
package main

import (
	"fmt"
	"log"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"syscall"
	"unsafe"
)

const (
	version     = "1.1.1"
	windowClass = "FancyBorderless"
)

var (
	app             *manager
	taskbarCreated  uintptr
	wndProcCallback = syscall.NewCallback(wndProc)
)

func main() {
	// Window messages, hooks and timers all belong to the thread that created them.
	runtime.LockOSThread()
	procSetProcessDpiAwarenessContext.Call(dpiAwarenessPerMonitorV2)

	if len(os.Args) > 1 {
		attachConsole()
		os.Exit(runCommand(os.Args[1]))
	}
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run() error {
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		return err
	}
	_, _, err := procCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(utf16Ptr(`Local\FancyBorderless`))))
	if err == syscall.Errno(errorAlreadyExists) {
		return nil
	}
	logFile, err := openLog()
	if err != nil {
		return err
	}
	defer logFile.Close()
	log.SetOutput(logFile)
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	app = newManager()
	if err := app.createWindow(); err != nil {
		return err
	}
	app.start()

	var msg winMsg
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
	log.Print("exited")
	return nil
}

func openLog() (*os.File, error) {
	if fi, err := os.Stat(logPath); err == nil && fi.Size() > 1<<20 {
		os.Rename(logPath, logPath+".old")
	}
	return os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
}

func (m *manager) createWindow() error {
	instance, _, _ := procGetModuleHandleW.Call(0)
	className := utf16Ptr(windowClass)
	wc := wndClassEx{WndProc: wndProcCallback, Instance: instance, ClassName: className}
	wc.Size = uint32(unsafe.Sizeof(wc))
	if r, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		return fmt.Errorf("RegisterClassEx: %w", err)
	}
	// A hidden top-level window rather than a message-only one: only top-level windows get the
	// WM_DISPLAYCHANGE and WM_SETTINGCHANGE broadcasts.
	h, _, err := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(className)),
		wsPopup, 0, 0, 0, 0, 0, 0, instance, 0)
	if h == 0 {
		return fmt.Errorf("CreateWindowEx: %w", err)
	}
	m.hwnd = h
	return nil
}

func (m *manager) start() {
	m.icon, _, _ = procLoadIconW.Call(0, idiApplication)
	taskbarCreated, _, _ = procRegisterWindowMessageW.Call(uintptr(unsafe.Pointer(utf16Ptr("TaskbarCreated"))))
	m.reloadConfig()
	m.addTrayIcon()
	m.hook()
	procSetTimer.Call(m.hwnd, timerTick, 100, 0)
	procSetTimer.Call(m.hwnd, timerScan, 1000, 0)
	log.Printf("FancyBorderless %s started", version)
	m.scan()
}

func wndProc(h, msg, wParam, lParam uintptr) uintptr {
	switch msg {
	case wmTimer:
		app.onTimer(wParam)
		return 0
	case wmHotkey:
		if wParam == hotkeyToggleTitleBar {
			app.toggleTitleBar()
		}
		return 0
	case wmTrayIcon:
		app.onTrayMessage(uint32(lParam))
		return 0
	case wmDisplayChange:
		app.scheduleRefit()
	case wmSettingChange:
		if wParam == spiSetWorkArea {
			app.scheduleRefit()
		}
	case wmClose:
		app.shutdown()
		procDestroyWindow.Call(h)
		return 0
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	default:
		// Explorer restarted; the tray icon has to be added again.
		if taskbarCreated != 0 && msg == taskbarCreated {
			app.addTrayIcon()
		}
	}
	r, _, _ := procDefWindowProcW.Call(h, msg, wParam, lParam)
	return r
}

// attachConsole lets the command-line options print when this GUI program is started from a
// terminal without redirected output.
func attachConsole() {
	if h, err := syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE); err == nil && h != 0 && h != syscall.InvalidHandle {
		return
	}
	if r, _, _ := procAttachConsole.Call(attachParentProcess); r == 0 {
		return
	}
	if f, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0); err == nil {
		os.Stdout, os.Stderr = f, f
	}
}

func runCommand(arg string) int {
	switch arg {
	case "--list":
		return list()
	case "--quit":
		h, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(utf16Ptr(windowClass))), 0)
		if h == 0 {
			fmt.Println("FancyBorderless isn't running")
			return 1
		}
		procPostMessageW.Call(h, wmClose, 0, 0)
		return 0
	case "--version":
		fmt.Println("FancyBorderless", version)
		return 0
	default:
		fmt.Println("usage: FancyBorderless [--list | --quit | --version]")
		return 2
	}
}

// list prints what FancyBorderless sees: monitors, the zones it calculates for them and the
// windows FancyZones has snapped.
func list() int {
	fz := newFancyZones()
	if err := fz.load(); err != nil {
		fmt.Println("FancyZones layouts:", err)
	}
	desktop := currentDesktop()
	fmt.Printf("FancyZones data: %s\nVirtual desktop: %s\n", fz.dir, desktop)

	for _, hmon := range allMonitors() {
		mon, ok := identifyMonitor(hmon)
		if !ok {
			continue
		}
		primary := ""
		if mon.primary {
			primary = " (primary)"
		}
		fmt.Printf("\nMonitor %s %s %s%s\n  screen %v, work area %v\n", mon.device, mon.pnp, mon.instance, primary, mon.bounds, mon.work)
		layout := fz.layoutFor(mon, desktop)
		if layout.err != nil {
			fmt.Printf("  %v\n", layout.err)
			continue
		}
		fmt.Printf("  layout %q\n", layout.name)
		for _, i := range slices.Sorted(maps.Keys(layout.zones)) {
			fmt.Printf("    zone %d: %v\n", i+1, layout.zones[i])
		}
	}

	fmt.Println("\nWindows FancyZones has snapped:")
	for _, h := range topLevelWindows() {
		bits := zoneBits(h)
		if bits == 0 || !isWindowVisible(h) {
			continue
		}
		exe := processPath(windowPID(h))
		style := windowStyle(h)
		titleBar := "drawn by Windows"
		if !drawsTitleBar(h) {
			if bar := ownTitleBarHeight(h); bar > 0 {
				titleBar = fmt.Sprintf("drawn by the app, %d px", bar)
			} else {
				titleBar = "none, or the app's own with controls in it"
			}
		}
		fmt.Printf("  %s %q zone %s\n    window %v, visible %v, resize border %v, popup %v\n    title bar: %s\n",
			filepath.Base(exe), windowTitle(h), zoneList(bits), windowRect(h), visibleRect(h),
			style&wsThickFrame != 0, style&wsPopup != 0, titleBar)
	}
	return 0
}
