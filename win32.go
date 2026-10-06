package main

import (
	"fmt"
	"slices"
	"syscall"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	dwmapi   = syscall.NewLazyDLL("dwmapi.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	shcore   = syscall.NewLazyDLL("shcore.dll")

	procAppendMenuW                   = user32.NewProc("AppendMenuW")
	procChangeWindowMessageFilterEx   = user32.NewProc("ChangeWindowMessageFilterEx")
	procClientToScreen                = user32.NewProc("ClientToScreen")
	procCreatePopupMenu               = user32.NewProc("CreatePopupMenu")
	procCreateWindowExW               = user32.NewProc("CreateWindowExW")
	procDefWindowProcW                = user32.NewProc("DefWindowProcW")
	procDestroyMenu                   = user32.NewProc("DestroyMenu")
	procDestroyWindow                 = user32.NewProc("DestroyWindow")
	procDispatchMessageW              = user32.NewProc("DispatchMessageW")
	procEnumDisplayDevicesW           = user32.NewProc("EnumDisplayDevicesW")
	procEnumDisplayMonitors           = user32.NewProc("EnumDisplayMonitors")
	procEnumWindows                   = user32.NewProc("EnumWindows")
	procFindWindowW                   = user32.NewProc("FindWindowW")
	procGetAncestor                   = user32.NewProc("GetAncestor")
	procGetClientRect                 = user32.NewProc("GetClientRect")
	procGetCursorPos                  = user32.NewProc("GetCursorPos")
	procGetDpiForWindow               = user32.NewProc("GetDpiForWindow")
	procGetForegroundWindow           = user32.NewProc("GetForegroundWindow")
	procGetMonitorInfoW               = user32.NewProc("GetMonitorInfoW")
	procGetPropW                      = user32.NewProc("GetPropW")
	procGetSystemMetrics              = user32.NewProc("GetSystemMetrics")
	procGetWindow                     = user32.NewProc("GetWindow")
	procGetWindowLongPtrW             = user32.NewProc("GetWindowLongPtrW")
	procGetWindowRect                 = user32.NewProc("GetWindowRect")
	procGetWindowRgnBox               = user32.NewProc("GetWindowRgnBox")
	procGetWindowTextW                = user32.NewProc("GetWindowTextW")
	procGetWindowThreadProcessId      = user32.NewProc("GetWindowThreadProcessId")
	procIsHungAppWindow               = user32.NewProc("IsHungAppWindow")
	procIsIconic                      = user32.NewProc("IsIconic")
	procIsWindow                      = user32.NewProc("IsWindow")
	procIsWindowVisible               = user32.NewProc("IsWindowVisible")
	procIsZoomed                      = user32.NewProc("IsZoomed")
	procKillTimer                     = user32.NewProc("KillTimer")
	procLoadIconW                     = user32.NewProc("LoadIconW")
	procLoadImageW                    = user32.NewProc("LoadImageW")
	procMessageBeep                   = user32.NewProc("MessageBeep")
	procMonitorFromWindow             = user32.NewProc("MonitorFromWindow")
	procMsgWaitForMultipleObjectsEx   = user32.NewProc("MsgWaitForMultipleObjectsEx")
	procPeekMessageW                  = user32.NewProc("PeekMessageW")
	procPhysicalToLogicalPointForPMD  = user32.NewProc("PhysicalToLogicalPointForPerMonitorDPI")
	procPostMessageW                  = user32.NewProc("PostMessageW")
	procPostQuitMessage               = user32.NewProc("PostQuitMessage")
	procRegisterClassExW              = user32.NewProc("RegisterClassExW")
	procRegisterHotKey                = user32.NewProc("RegisterHotKey")
	procRegisterWindowMessageW        = user32.NewProc("RegisterWindowMessageW")
	procRemovePropW                   = user32.NewProc("RemovePropW")
	procSendMessageTimeoutW           = user32.NewProc("SendMessageTimeoutW")
	procSetForegroundWindow           = user32.NewProc("SetForegroundWindow")
	procSetProcessDpiAwarenessContext = user32.NewProc("SetProcessDpiAwarenessContext")
	procSetPropW                      = user32.NewProc("SetPropW")
	procSetTimer                      = user32.NewProc("SetTimer")
	procSetWinEventHook               = user32.NewProc("SetWinEventHook")
	procSetWindowLongPtrW             = user32.NewProc("SetWindowLongPtrW")
	procSetWindowPos                  = user32.NewProc("SetWindowPos")
	procSetWindowRgn                  = user32.NewProc("SetWindowRgn")
	procTrackPopupMenu                = user32.NewProc("TrackPopupMenu")
	procTranslateMessage              = user32.NewProc("TranslateMessage")
	procUnhookWinEvent                = user32.NewProc("UnhookWinEvent")
	procUnregisterHotKey              = user32.NewProc("UnregisterHotKey")

	procAttachConsole               = kernel32.NewProc("AttachConsole")
	procCreateMutexW                = kernel32.NewProc("CreateMutexW")
	procFindFirstChangeNotification = kernel32.NewProc("FindFirstChangeNotificationW")
	procFindNextChangeNotification  = kernel32.NewProc("FindNextChangeNotification")
	procGetModuleHandleW            = kernel32.NewProc("GetModuleHandleW")
	procQueryFullProcessImageNameW  = kernel32.NewProc("QueryFullProcessImageNameW")
	procSetLastError                = kernel32.NewProc("SetLastError")

	procCreateRectRgn = gdi32.NewProc("CreateRectRgn")
	procDeleteObject  = gdi32.NewProc("DeleteObject")

	procDwmGetWindowAttribute = dwmapi.NewProc("DwmGetWindowAttribute")
	procDwmSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute")
	procGetDpiForMonitor      = shcore.NewProc("GetDpiForMonitor")

	procShellExecuteW    = shell32.NewProc("ShellExecuteW")
	procShellNotifyIconW = shell32.NewProc("Shell_NotifyIconW")
)

const (
	wsPopup      = 0x80000000
	wsChild      = 0x40000000
	wsCaption    = 0x00C00000
	wsThickFrame = 0x00040000
	frameStyles  = wsCaption | wsThickFrame

	wsExDlgModalFrame = 0x00000001
	wsExToolWindow    = 0x00000080
	wsExWindowEdge    = 0x00000100
	wsExClientEdge    = 0x00000200
	wsExStaticEdge    = 0x00020000
	frameExStyles     = wsExDlgModalFrame | wsExWindowEdge | wsExClientEdge | wsExStaticEdge

	swpNoZOrder      = 0x0004
	swpNoActivate    = 0x0010
	swpFrameChanged  = 0x0020
	swpNoOwnerZOrder = 0x0200
	swpQuiet         = swpNoZOrder | swpNoActivate | swpNoOwnerZOrder

	swShowNormal = 1
	gaRoot       = 2
	gwOwner      = 4

	monitorDefaultToNearest   = 2
	monitorInfoPrimary        = 1
	eddGetDeviceInterfaceName = 1
	displayDeviceActive       = 1

	dwmwaNCRenderingEnabled  = 1
	dwmwaNCRenderingPolicy   = 2
	dwmwaExtendedFrameBounds = 9
	dwmwaCloaked             = 14
	dwmwaSystemBackdropType  = 38
	dwmncrpDisabled          = 1
	dwmncrpEnabled           = 2
	dwmsbtNone               = 1

	processQueryLimitedInformation = 0x1000

	eventSystemMoveSizeEnd    = 0x000B
	eventObjectDestroy        = 0x8001
	eventObjectShow           = 0x8002
	eventObjectLocationChange = 0x800B
	winEventOutOfContext      = 0x0000
	winEventSkipOwnProcess    = 0x0002
	objidWindow               = 0
	childidSelf               = 0

	wmNull          = 0x0000
	wmDestroy       = 0x0002
	wmClose         = 0x0010
	wmSettingChange = 0x001A
	wmContextMenu   = 0x007B
	wmDisplayChange = 0x007E
	wmDpiChanged    = 0x02E0
	wmNcHitTest     = 0x0084
	wmTimer         = 0x0113
	wmHotkey        = 0x0312
	wmLButtonUp     = 0x0202
	wmRButtonUp     = 0x0205
	wmThemeChanged  = 0x031A
	wmApp           = 0x8000
	spiSetWorkArea  = 0x002F

	htCaption       = 2
	htMinButton     = 8
	htMaxButton     = 9
	htClose         = 20
	smtoAbortIfHung = 0x0002
	msgfltAllow     = 1

	wmQuit             = 0x0012
	pmRemove           = 0x0001
	qsAllInput         = 0x04FF
	mwmoInputAvailable = 0x0004
	infinite           = 0xFFFFFFFF
	waitFailed         = 0xFFFFFFFF

	fileNotifyChangeFileName  = 0x01
	fileNotifyChangeLastWrite = 0x10

	modAlt      = 0x0001
	modControl  = 0x0002
	modShift    = 0x0004
	modWin      = 0x0008
	modNoRepeat = 0x4000

	nimAdd     = 0
	nimModify  = 1
	nimDelete  = 2
	nifMessage = 0x1
	nifIcon    = 0x2
	nifTip     = 0x4
	nifInfo    = 0x10

	niifInfo    = 0x1
	niifNoSound = 0x10

	mfString    = 0x0000
	mfGrayed    = 0x0001
	mfChecked   = 0x0008
	mfPopup     = 0x0010
	mfSeparator = 0x0800

	tpmRightButton = 0x0002
	tpmNoNotify    = 0x0080
	tpmReturnCmd   = 0x0100

	idiApplication     = 32512
	imageIcon          = 1
	smCxSmIcon         = 49
	errorAlreadyExists = 183
)

// Negative values from the Windows headers, as two's-complement uintptr.
const (
	gwlStyle                 = ^uintptr(15) // GWL_STYLE (-16)
	gwlExStyle               = ^uintptr(19) // GWL_EXSTYLE (-20)
	dpiAwarenessPerMonitorV2 = ^uintptr(3)  // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 (-4)
	attachParentProcess      = ^uintptr(0)  // ATTACH_PARENT_PROCESS (-1)
)

type rect struct{ Left, Top, Right, Bottom int32 }

func (r rect) width() int32  { return r.Right - r.Left }
func (r rect) height() int32 { return r.Bottom - r.Top }
func (r rect) String() string {
	return fmt.Sprintf("%d,%d %dx%d", r.Left, r.Top, r.width(), r.height())
}

func unionRect(a, b rect) rect {
	return rect{min(a.Left, b.Left), min(a.Top, b.Top), max(a.Right, b.Right), max(a.Bottom, b.Bottom)}
}

func nearRect(a, b rect, tolerance int32) bool {
	for _, delta := range [...]int32{a.Left - b.Left, a.Top - b.Top, a.Right - b.Right, a.Bottom - b.Bottom} {
		if delta < -tolerance || delta > tolerance {
			return false
		}
	}
	return true
}

type point struct{ X, Y int32 }

type winMsg struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
	Private uint32
}

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}

type monitorInfoEx struct {
	Size    uint32
	Monitor rect
	Work    rect
	Flags   uint32
	Device  [32]uint16
}

type displayDevice struct {
	Size         uint32
	DeviceName   [32]uint16
	DeviceString [128]uint16
	StateFlags   uint32
	DeviceID     [128]uint16
	DeviceKey    [128]uint16
}

type notifyIconData struct {
	Size            uint32
	Wnd             uintptr
	ID              uint32
	Flags           uint32
	CallbackMessage uint32
	Icon            uintptr
	Tip             [128]uint16
	State           uint32
	StateMask       uint32
	Info            [256]uint16
	Version         uint32
	InfoTitle       [64]uint16
	InfoFlags       uint32
	GUIDItem        [16]byte
	BalloonIcon     uintptr
}

func utf16Ptr(s string) *uint16 {
	p, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		panic(err) // only called with literals and paths, which never contain NUL
	}
	return p
}

func isWindow(h uintptr) bool        { r, _, _ := procIsWindow.Call(h); return r != 0 }
func isWindowVisible(h uintptr) bool { r, _, _ := procIsWindowVisible.Call(h); return r != 0 }
func isIconic(h uintptr) bool        { r, _, _ := procIsIconic.Call(h); return r != 0 }
func isZoomed(h uintptr) bool        { r, _, _ := procIsZoomed.Call(h); return r != 0 }
func rootWindow(h uintptr) uintptr   { r, _, _ := procGetAncestor.Call(h, gaRoot); return r }
func foregroundWindow() uintptr      { r, _, _ := procGetForegroundWindow.Call(); return r }

// frameWindow finds the frame around a separate, owned content surface. Some
// players put focus and FancyZones' marker on their video window, although its
// title bar belongs to the owner. Ordinary dialogs and floating windows stay
// independent: only an unframed surface filling the owner's client area qualifies.
func frameWindow(h uintptr) uintptr {
	h = rootWindow(h)
	if h == 0 || windowStyle(h)&frameStyles != 0 || windowExStyle(h)&(wsExToolWindow|wsExDlgModalFrame) != 0 ||
		getProp(h, propSavedStyle) != 0 || getProp(h, propHidden) != 0 {
		return h
	}
	owner, _, _ := procGetWindow.Call(h, gwOwner)
	if owner == 0 || windowPID(owner) != windowPID(h) || windowExStyle(owner)&wsExToolWindow != 0 {
		return h
	}
	if windowStyle(owner)&frameStyles == 0 && getProp(owner, propSavedStyle) == 0 && getProp(owner, propHidden) == 0 {
		return h
	}
	content, client := windowRect(h), clientScreenRect(owner)
	tolerance := scaleForWindow(owner, 8)
	near := func(a, b int32) bool { return a-b >= -tolerance && a-b <= tolerance }
	if !near(content.Left, client.Left) || !near(content.Right, client.Right) || !near(content.Bottom, client.Bottom) ||
		content.Top < client.Top-tolerance || content.Top > client.Top+scaleForWindow(owner, 80) {
		return h
	}
	return owner
}

// isHung reports a window that hasn't answered Windows for a few seconds. Changing its style
// or position would wait for it, and FancyBorderless with it.
func isHung(h uintptr) bool { r, _, _ := procIsHungAppWindow.Call(h); return r != 0 }

// clipTo shows only r (screen coordinates) of a window: the rest isn't drawn, and clicks
// there go to whatever is below. Window regions are in the window's own coordinates, which
// are logical pixels for apps that Windows scales for display scaling, hence the conversion.
func clipTo(h uintptr, r rect) error {
	win := windowRect(h)
	p := [3]point{{win.Left, win.Top}, {r.Left, r.Top}, {r.Right - 1, r.Bottom - 1}}
	for i := range p {
		procPhysicalToLogicalPointForPMD.Call(h, uintptr(unsafe.Pointer(&p[i])))
	}
	want := rect{p[1].X - p[0].X, p[1].Y - p[0].Y, p[2].X - p[0].X + 1, p[2].Y - p[0].Y + 1}
	if have, ok := regionBox(h); ok && have == want {
		return nil
	}
	rgn, _, _ := procCreateRectRgn.Call(uintptr(want.Left), uintptr(want.Top), uintptr(want.Right), uintptr(want.Bottom))
	if ok, _, err := procSetWindowRgn.Call(h, rgn, 1); ok == 0 {
		procDeleteObject.Call(rgn)
		return err
	}
	return nil
}

func unclipWindow(h uintptr) { procSetWindowRgn.Call(h, 0, 1) }

// regionBox is the bounding box of a window's region; false if it has none.
func regionBox(h uintptr) (rect, bool) {
	var r rect
	kind, _, _ := procGetWindowRgnBox.Call(h, uintptr(unsafe.Pointer(&r)))
	return r, kind != 0 // ERROR (0) means no region
}

func hasRegion(h uintptr) bool { _, ok := regionBox(h); return ok }

// setFrameDrawing turns the frame Windows draws around a window off or back on. Windows draws
// it on top of any region; switched off, the frame is part of the window and gets clipped.
func setFrameDrawing(h uintptr, on bool) {
	policy := uint32(dwmncrpDisabled)
	if on {
		// USEWINDOWSTYLE can leave custom frames disabled, exposing their thick resize
		// borders. Restore rendering explicitly when it was enabled before clipping.
		policy = dwmncrpEnabled
	}
	procDwmSetWindowAttribute.Call(h, dwmwaNCRenderingPolicy, uintptr(unsafe.Pointer(&policy)), unsafe.Sizeof(policy))
	if on && !frameDrawn(h) {
		// Some apps cache the unthemed frame while clipped. Changing the DWM policy alone
		// leaves their resize border exposed until the app refreshes its theme.
		var result uintptr
		procSendMessageTimeoutW.Call(h, wmThemeChanged, 0, 0, smtoAbortIfHung, 100, uintptr(unsafe.Pointer(&result)))
	}
}

func frameDrawn(h uintptr) bool {
	var on int32
	procDwmGetWindowAttribute.Call(h, dwmwaNCRenderingEnabled, uintptr(unsafe.Pointer(&on)), unsafe.Sizeof(on))
	return on != 0
}

// backdrop is the material Windows 11 draws behind a window, like Mica. A region doesn't clip
// it either, so a clipped window gets none while it's clipped.
func backdrop(h uintptr) uint32 {
	var v uint32
	procDwmGetWindowAttribute.Call(h, dwmwaSystemBackdropType, uintptr(unsafe.Pointer(&v)), unsafe.Sizeof(v))
	return v
}

func setBackdrop(h uintptr, v uint32) {
	procDwmSetWindowAttribute.Call(h, dwmwaSystemBackdropType, uintptr(unsafe.Pointer(&v)), unsafe.Sizeof(v))
}

func windowStyle(h uintptr) uint32 {
	r, _, _ := procGetWindowLongPtrW.Call(h, gwlStyle)
	return uint32(r)
}

func windowExStyle(h uintptr) uint32 {
	r, _, _ := procGetWindowLongPtrW.Call(h, gwlExStyle)
	return uint32(r)
}

// setWindowLong clears the last error first: SetWindowLongPtr returns the previous value,
// which can legitimately be zero, so zero alone doesn't mean failure.
func setWindowLong(h, index uintptr, value uint32) error {
	procSetLastError.Call(0)
	r, _, err := procSetWindowLongPtrW.Call(h, index, uintptr(value))
	if r == 0 && err != syscall.Errno(0) {
		return err
	}
	return nil
}

func setStyles(h uintptr, style, exStyle uint32) error {
	if err := setWindowLong(h, gwlStyle, style); err != nil {
		return err
	}
	return setWindowLong(h, gwlExStyle, exStyle)
}

func windowRect(h uintptr) rect {
	var r rect
	procGetWindowRect.Call(h, uintptr(unsafe.Pointer(&r)))
	return r
}

func clientSize(h uintptr) (int32, int32) {
	var r rect
	procGetClientRect.Call(h, uintptr(unsafe.Pointer(&r)))
	return r.width(), r.height()
}

// clientScreenRect is the client area in screen coordinates.
func clientScreenRect(h uintptr) rect {
	var origin point
	procClientToScreen.Call(h, uintptr(unsafe.Pointer(&origin)))
	w, ht := clientSize(h)
	return rect{origin.X, origin.Y, origin.X + w, origin.Y + ht}
}

// drawsTitleBar reports whether Windows draws a title bar above the window's content.
// Browsers, Explorer and most modern apps set the caption style but draw their own bar
// inside their content, so their content starts at the very top of the window.
func drawsTitleBar(h uintptr) bool {
	const minTitleBar = 8
	return windowStyle(h)&wsCaption == wsCaption && clientScreenRect(h).Top-visibleRect(h).Top > minTitleBar
}

// hitTest asks the window what is at a screen point, the question Windows asks to know where
// a window can be dragged. It has no side effects.
func hitTest(h uintptr, x, y int32) (uintptr, bool) {
	var result uintptr
	lParam := uintptr(uint16(y))<<16 | uintptr(uint16(x))
	r, _, _ := procSendMessageTimeoutW.Call(h, wmNcHitTest, 0, lParam, smtoAbortIfHung, 100, uintptr(unsafe.Pointer(&result)))
	return result, r != 0
}

// ownTitleBar measures a title bar that an app draws itself. Apps answer hitTest with
// "caption" for its empty parts, so it usually ends where the caption in the middle ends.
// When that is only a thin strip (or a tab), the close button tells, which apps that draw
// their own answer with its button code. busy means the bar holds more than a title: tabs,
// search boxes and other controls answer "content". A plain bar is caption all along its
// bottom edge, from near the left to past the middle, while a browser's first tab always
// starts within a few dozen pixels of the left. A height of zero means none was found.
func ownTitleBar(h uintptr) (height int32, busy bool) {
	maxBar, minBar := scaleForWindow(h, 80), scaleForWindow(h, 16)
	r := visibleRect(h)
	lowest := func(x int32, codes ...uintptr) (int32, bool) {
		bottom := int32(-1)
		for y := int32(0); y < maxBar; y++ {
			hit, ok := hitTest(h, x, r.Top+y)
			if !ok {
				return 0, false
			}
			if slices.Contains(codes, hit) {
				bottom = y
			}
		}
		return bottom + 1, true
	}
	height, ok := lowest(r.Left+r.width()*45/100, htCaption)
	if ok && height < minBar {
		height, ok = lowest(r.Right-scaleForWindow(h, 20), htMinButton, htMaxButton, htClose)
	}
	if !ok || height < minBar {
		return 0, false
	}
	columns := []int32{r.Left + r.width()*15/100, r.Left + r.width()*30/100, r.Left + r.width()*60/100}
	for _, px := range []int32{60, 120, 180, 240} {
		if x := scaleForWindow(h, px); x < r.width()/2 {
			columns = append(columns, r.Left+x)
		}
	}
	for _, x := range columns {
		last, ok1 := hitTest(h, x, r.Top+height-1)
		below, ok2 := hitTest(h, x, r.Top+height)
		if !ok1 || !ok2 {
			return 0, false
		}
		if last != htCaption || below == htCaption {
			return height, true
		}
	}
	return height, false
}

// visibleRect is the window as drawn, without the invisible resize borders that framed
// windows have on Windows 10 and 11. FancyZones fits this rectangle to the zone.
func visibleRect(h uintptr) rect {
	var r rect
	hr, _, _ := procDwmGetWindowAttribute.Call(h, dwmwaExtendedFrameBounds, uintptr(unsafe.Pointer(&r)), unsafe.Sizeof(r))
	if hr != 0 {
		return windowRect(h)
	}
	return r
}

func isCloaked(h uintptr) bool {
	var v uint32
	hr, _, _ := procDwmGetWindowAttribute.Call(h, dwmwaCloaked, uintptr(unsafe.Pointer(&v)), unsafe.Sizeof(v))
	return hr == 0 && v != 0
}

// scaleForWindow converts a size in pixels at 100 % scaling to the window's monitor.
func scaleForWindow(h uintptr, px int32) int32 {
	return px * int32(monitorDPI(monitorFromWindow(h))) / 96
}

func monitorDPI(hmon uintptr) uint32 {
	var x, y uint32
	if hr, _, _ := procGetDpiForMonitor.Call(hmon, 0, uintptr(unsafe.Pointer(&x)), uintptr(unsafe.Pointer(&y))); hr != 0 || x == 0 {
		return 96
	}
	return x
}

// DPI changes on a secondary monitor need not reach our hidden window.
func monitorStamp() string {
	stamp := ""
	for _, hmon := range allMonitors() {
		if mi, ok := monitorInfo(hmon); ok {
			stamp += fmt.Sprintf("%x:%v:%v:%d;", hmon, mi.Monitor, mi.Work, monitorDPI(hmon))
		}
	}
	return stamp
}

func setWindowPos(h uintptr, r rect, flags uintptr) error {
	ok, _, err := procSetWindowPos.Call(h, 0, uintptr(r.Left), uintptr(r.Top), uintptr(r.width()), uintptr(r.height()), flags)
	if ok == 0 {
		return err
	}
	return nil
}

// setVisibleRect places a framed window so its visible part covers r, the way FancyZones
// does, by growing the window by its invisible borders.
func setVisibleRect(h uintptr, r rect) error {
	if err := setWindowPos(h, r, swpQuiet|swpFrameChanged); err != nil {
		return err
	}
	frame, win := visibleRect(h), windowRect(h)
	adjusted := rect{
		Left:   r.Left - (frame.Left - win.Left),
		Top:    r.Top - (frame.Top - win.Top),
		Right:  r.Right + (win.Right - frame.Right),
		Bottom: r.Bottom + (win.Bottom - frame.Bottom),
	}
	if adjusted == r {
		return nil
	}
	return setWindowPos(h, adjusted, swpQuiet)
}

func windowPID(h uintptr) uint32 {
	var pid uint32
	procGetWindowThreadProcessId.Call(h, uintptr(unsafe.Pointer(&pid)))
	return pid
}

func windowTitle(h uintptr) string {
	buf := make([]uint16, 256)
	n, _, _ := procGetWindowTextW.Call(h, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return syscall.UTF16ToString(buf[:n])
}

// processPath opens the process with PROCESS_QUERY_LIMITED_INFORMATION only: a handle that
// can't read, write or inject into it.
func processPath(pid uint32) string {
	h, err := syscall.OpenProcess(processQueryLimitedInformation, false, pid)
	if err != nil {
		return ""
	}
	defer syscall.CloseHandle(h)
	buf := make([]uint16, 1024)
	n := uint32(len(buf))
	r, _, _ := procQueryFullProcessImageNameW.Call(uintptr(h), 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n)))
	if r == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:n])
}

var propZones = utf16Ptr("FancyZones_zones")

// zoneBits reads the property FancyZones stamps on every window it snaps: bit n set means
// the window is in zone n+1 of the layout on its monitor. Zero means not snapped.
func zoneBits(h uintptr) uint64 {
	r, _, _ := procGetPropW.Call(h, uintptr(unsafe.Pointer(propZones)))
	return uint64(r)
}

// Our own marks on the windows we change. Window properties live with the window, so a new
// FancyBorderless instance can still undo the changes after the old one was killed.
var (
	propSavedStyle = utf16Ptr("FancyBorderless_style") // original style << 32 | ex-style
	// Hidden state: bit 0 marks clipping, bit 1 an originally undrawn frame, the rest the
	// original backdrop. The old value 1 restores a drawn frame and the default backdrop.
	propHidden     = utf16Ptr("FancyBorderless_hidden")
	propKeepsFrame = utf16Ptr("FancyBorderless_keepsframe") // the program won't do without its frame
)

func getProp(h uintptr, name *uint16) uintptr {
	r, _, _ := procGetPropW.Call(h, uintptr(unsafe.Pointer(name)))
	return r
}

func setProp(h uintptr, name *uint16, value uintptr) {
	procSetPropW.Call(h, uintptr(unsafe.Pointer(name)), value)
}

func removeProp(h uintptr, name *uint16) {
	procRemovePropW.Call(h, uintptr(unsafe.Pointer(name)))
}

func monitorFromWindow(h uintptr) uintptr {
	r, _, _ := procMonitorFromWindow.Call(h, monitorDefaultToNearest)
	return r
}

func monitorInfo(hmon uintptr) (monitorInfoEx, bool) {
	mi := monitorInfoEx{}
	mi.Size = uint32(unsafe.Sizeof(mi))
	r, _, _ := procGetMonitorInfoW.Call(hmon, uintptr(unsafe.Pointer(&mi)))
	return mi, r != 0
}

// The enumeration callbacks run synchronously on the calling thread, so collecting into a
// package variable is safe.
var (
	enumResult          []uintptr
	enumWindowsCallback = syscall.NewCallback(func(h, _ uintptr) uintptr {
		enumResult = append(enumResult, h)
		return 1
	})
	enumMonitorsCallback = syscall.NewCallback(func(hmon, _, _, _ uintptr) uintptr {
		enumResult = append(enumResult, hmon)
		return 1
	})
)

func topLevelWindows() []uintptr {
	enumResult = enumResult[:0] // reuse the buffer; callers don't hold on to it
	procEnumWindows.Call(enumWindowsCallback, 0)
	return enumResult
}

func allMonitors() []uintptr {
	enumResult = enumResult[:0]
	procEnumDisplayMonitors.Call(0, 0, enumMonitorsCallback, 0)
	return enumResult
}

// watchFolder returns a handle Windows signals when a file in dir is written or replaced.
func watchFolder(dir string) (syscall.Handle, bool) {
	h, _, _ := procFindFirstChangeNotification.Call(uintptr(unsafe.Pointer(utf16Ptr(dir))), 0,
		fileNotifyChangeLastWrite|fileNotifyChangeFileName)
	return syscall.Handle(h), syscall.Handle(h) != syscall.InvalidHandle && h != 0
}

func openFile(path string) {
	procShellExecuteW.Call(0, uintptr(unsafe.Pointer(utf16Ptr("open"))), uintptr(unsafe.Pointer(utf16Ptr(path))), 0, 0, swShowNormal)
}

func beep() { procMessageBeep.Call(0xFFFFFFFF) }
