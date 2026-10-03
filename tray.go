package main

import (
	"log"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"unsafe"
)

const wmTrayIcon = wmApp + 1

const (
	menuEnabled = iota + 1
	menuStartup
	menuAdmin
	menuSettings
	menuLog
	menuExit
	menuFirstApp = 100
)

func (m *manager) trayData(flags uint32) *notifyIconData {
	nid := &notifyIconData{Wnd: m.hwnd, ID: 1, Flags: flags, CallbackMessage: wmTrayIcon, Icon: m.icon}
	nid.Size = uint32(unsafe.Sizeof(*nid))
	tip := "FancyBorderless"
	if !m.cfg.RemoveTitleBars {
		tip += " (off)"
	}
	copyUTF16(nid.Tip[:], tip)
	return nid
}

func copyUTF16(dst []uint16, s string) {
	if t, err := syscall.UTF16FromString(s); err == nil {
		copy(dst[:len(dst)-1], t)
	}
}

func (m *manager) addTrayIcon() {
	procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(m.trayData(nifMessage|nifIcon|nifTip))))
}

func (m *manager) updateTrayIcon() {
	procShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(m.trayData(nifTip))))
}

func (m *manager) removeTrayIcon() {
	procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(m.trayData(0))))
}

// notify shows a short Windows notification from the tray icon, so hotkey presses always get
// visible feedback.
func (m *manager) notify(text string) {
	nid := m.trayData(nifInfo)
	copyUTF16(nid.Info[:], text)
	copyUTF16(nid.InfoTitle[:], "FancyBorderless")
	nid.InfoFlags = niifInfo | niifNoSound
	procShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(nid)))
	log.Print(text)
}

func (m *manager) onTrayMessage(event uint32) {
	switch event {
	case wmLButtonUp, wmRButtonUp, wmContextMenu:
		m.showMenu()
	}
}

// menuApps lists the apps the "Keep title bar for" submenu offers: those the user chose for
// and those with a snapped window that has a title bar.
func (m *manager) menuApps() []string {
	seen := map[string]string{}
	add := func(exeName string) {
		key := strings.ToLower(displayName(exeName))
		if key != "" && seen[key] == "" {
			seen[key] = exeName
		}
	}
	for _, e := range slices.Concat(m.cfg.KeepTitleBarApps, m.cfg.RemoveTitleBarApps) {
		add(strings.TrimSpace(e))
	}
	for h, w := range m.windows {
		if (w.titleBar || w.ownBar > 0) && w.exe != "" && isWindow(h) {
			add(filepath.Base(w.exe))
		}
	}
	apps := make([]string, 0, len(seen))
	for _, exeName := range seen {
		apps = append(apps, exeName)
	}
	slices.SortFunc(apps, func(a, b string) int { return strings.Compare(strings.ToLower(a), strings.ToLower(b)) })
	return apps
}

func (m *manager) showMenu() {
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer procDestroyMenu.Call(menu)
	add := func(menu uintptr, flags uint32, id uintptr, text string) {
		procAppendMenuW.Call(menu, uintptr(flags), id, uintptr(unsafe.Pointer(utf16Ptr(text))))
	}
	checked := func(on bool) uint32 {
		if on {
			return mfString | mfChecked
		}
		return mfString
	}

	apps := m.menuApps()
	appsMenu, _, _ := procCreatePopupMenu.Call()
	if len(apps) == 0 {
		add(appsMenu, mfString|mfGrayed, 0, "Snap an app with FancyZones first")
	}
	for i, exeName := range apps {
		add(appsMenu, checked(m.keepsTitleBar(exeName, m.hasBusyBar(exeName))), uintptr(menuFirstApp+i), displayName(exeName))
	}

	add(menu, mfString|mfGrayed, 0, "FancyBorderless "+version)
	add(menu, mfSeparator, 0, "")
	add(menu, checked(m.cfg.RemoveTitleBars), menuEnabled, "Remove title bars")
	add(menu, checked(startsWithWindows()), menuStartup, "Start with Windows")
	add(menu, checked(m.cfg.RunAsAdministrator), menuAdmin, "Run as administrator")
	add(menu, mfPopup, appsMenu, "Keep title bar for") // the menu owns and destroys appsMenu
	add(menu, mfSeparator, 0, "")
	add(menu, mfString, menuSettings, "Open settings file")
	add(menu, mfString, menuLog, "Open log")
	add(menu, mfSeparator, 0, "")
	add(menu, mfString, menuExit, "Exit")

	var pt point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	// TrackPopupMenu docs: the owner must be in the foreground or the menu won't close when
	// clicking elsewhere, and a posted message afterwards keeps it from closing instantly
	// the next time.
	procSetForegroundWindow.Call(m.hwnd)
	cmd, _, _ := procTrackPopupMenu.Call(menu, tpmReturnCmd|tpmRightButton|tpmNoNotify, uintptr(pt.X), uintptr(pt.Y), 0, m.hwnd, 0)
	procPostMessageW.Call(m.hwnd, wmNull, 0, 0)

	switch {
	case cmd == menuEnabled:
		m.setEnabled(!m.cfg.RemoveTitleBars)
	case cmd == menuStartup:
		m.toggleStartWithWindows()
	case cmd == menuAdmin:
		m.setRunAsAdministrator(!m.cfg.RunAsAdministrator)
	case cmd == menuSettings:
		openFile(configPath)
	case cmd == menuLog:
		openFile(logPath)
	case cmd == menuExit:
		procPostMessageW.Call(m.hwnd, wmClose, 0, 0)
	case cmd >= menuFirstApp && int(cmd-menuFirstApp) < len(apps):
		exeName := apps[cmd-menuFirstApp]
		m.setKeep(exeName, !m.keepsTitleBar(exeName, m.hasBusyBar(exeName)))
	}
}
