package main

import (
	"syscall"
	"unsafe"
)

const wmTrayIcon = wmApp + 1

const (
	menuPause = iota + 1
	menuReload
	menuSettings
	menuLog
	menuExit
)

func (m *manager) trayData(flags uint32) *notifyIconData {
	nid := &notifyIconData{Wnd: m.hwnd, ID: 1, Flags: flags, CallbackMessage: wmTrayIcon, Icon: m.icon}
	nid.Size = uint32(unsafe.Sizeof(*nid))
	tip := "FancyBorderless"
	if m.paused {
		tip += " (paused)"
	}
	if t, err := syscall.UTF16FromString(tip); err == nil {
		copy(nid.Tip[:len(nid.Tip)-1], t)
	}
	return nid
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

func (m *manager) onTrayMessage(event uint32) {
	switch event {
	case wmRButtonUp, wmContextMenu:
		m.showMenu()
	case wmLButtonDblClk:
		openFile(configPath)
	}
}

func (m *manager) showMenu() {
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer procDestroyMenu.Call(menu)
	add := func(flags uint32, id int, text string) {
		procAppendMenuW.Call(menu, uintptr(flags), uintptr(id), uintptr(unsafe.Pointer(utf16Ptr(text))))
	}
	pause := uint32(mfString)
	if m.paused {
		pause |= mfChecked
	}
	add(mfString|mfGrayed, 0, "FancyBorderless "+version)
	add(mfSeparator, 0, "")
	add(pause, menuPause, "Paused")
	add(mfString, menuReload, "Reload settings and layouts")
	add(mfString, menuSettings, "Open settings file")
	add(mfString, menuLog, "Open log")
	add(mfSeparator, 0, "")
	add(mfString, menuExit, "Exit")

	var pt point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	// TrackPopupMenu docs: the owner must be in the foreground or the menu won't close when
	// clicking elsewhere, and a posted message afterwards keeps it from closing instantly
	// the next time.
	procSetForegroundWindow.Call(m.hwnd)
	cmd, _, _ := procTrackPopupMenu.Call(menu, tpmReturnCmd|tpmRightButton|tpmNoNotify, uintptr(pt.X), uintptr(pt.Y), 0, m.hwnd, 0)
	procPostMessageW.Call(m.hwnd, wmNull, 0, 0)

	switch cmd {
	case menuPause:
		m.setPaused(!m.paused)
	case menuReload:
		m.cfgStamp, m.fzStamp = "", ""
		m.scan()
	case menuSettings:
		openFile(configPath)
	case menuLog:
		openFile(logPath)
	case menuExit:
		procPostMessageW.Call(m.hwnd, wmClose, 0, 0)
	}
}
