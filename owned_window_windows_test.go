package main

import (
	"runtime"
	"testing"
	"unsafe"
)

// Real, hidden native windows reproduce a player with an owned video surface.
// They are never candidates for the running utility and use no user settings.
func ownedTestWindows(t *testing.T) (uintptr, uintptr) {
	t.Helper()
	runtime.LockOSThread()
	t.Cleanup(runtime.UnlockOSThread)
	procSetProcessDpiAwarenessContext.Call(dpiAwarenessPerMonitorV2)
	class := utf16Ptr("STATIC")
	create := func(style, owner uintptr, r rect) uintptr {
		h, _, err := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(class)), 0, style,
			uintptr(r.Left), uintptr(r.Top), uintptr(r.width()), uintptr(r.height()), owner, 0, 0, 0)
		if h == 0 {
			t.Fatal(err)
		}
		t.Cleanup(func() { procDestroyWindow.Call(h) })
		return h
	}
	owner := create(wsCaption|wsThickFrame, 0, rect{300, 300, 1100, 900})
	r := clientScreenRect(owner)
	r.Left += 2
	r.Top += 32
	r.Right -= 2
	r.Bottom -= 2
	// No WS_CHILD or WS_POPUP: like SFVIP, this has an owner but GA_ROOT
	// and GA_ROOTOWNER both return the surface itself.
	surface := create(0, owner, r)
	// Windows supplies a caption when creating an overlapped window. The player
	// removes it after creation while retaining the ownership relationship.
	if err := setStyles(surface, windowStyle(surface)&^frameStyles, windowExStyle(surface)); err != nil {
		t.Fatal(err)
	}
	if err := setWindowPos(surface, r, swpQuiet|swpFrameChanged); err != nil {
		t.Fatal(err)
	}
	return owner, surface
}

func TestOwnedContentTargetsOuterFrame(t *testing.T) {
	owner, surface := ownedTestWindows(t)
	if rootWindow(surface) != surface {
		t.Fatal("fixture did not reproduce the separate video window")
	}
	if got := frameWindow(surface); got != owner {
		t.Fatalf("video targets %d, want outer frame %d", got, owner)
	}
	if got := frameWindow(owner); got != owner {
		t.Fatalf("outer frame targets %d, want itself", got)
	}
	// An outer frame that we stripped must still be selected for restoration.
	style := windowStyle(owner)
	setProp(owner, propSavedStyle, uintptr(style)<<32)
	if err := setStyles(owner, style&^frameStyles, windowExStyle(owner)); err != nil {
		t.Fatal(err)
	}
	if got := frameWindow(surface); got != owner {
		t.Fatalf("stripped frame targets %d, want %d", got, owner)
	}
}

func TestOwnedContentSharesZoneMarker(t *testing.T) {
	owner, surface := ownedTestWindows(t)
	m := newManager()
	w := m.track(owner)
	w.zoneSource = surface
	bounds := visibleRect(owner)
	shift := func(r rect) rect { return rect{r.Left + 900, r.Top, r.Right + 900, r.Bottom} }
	m.layouts[monitorFromWindow(owner)] = zoneLayout{zones: map[int]rect{2: bounds, 1: shift(bounds)}}
	setProp(surface, propZones, 4)
	if zoneBits(owner) != 0 || m.zoneBits(owner) != 4 {
		t.Fatal("outer frame did not use the video's zone marker")
	}
	setProp(surface, propZones, 2)
	if m.zoneBits(owner) != 0 {
		t.Fatal("a stale video marker was accepted while the frame was elsewhere")
	}
	for _, h := range []uintptr{owner, surface} {
		if err := setWindowPos(h, shift(windowRect(h)), swpQuiet); err != nil {
			t.Fatal(err)
		}
	}
	if m.zoneBits(owner) != 2 {
		t.Fatal("outer frame did not follow a zone change")
	}
	m.layouts[monitorFromWindow(owner)].zones[0] = visibleRect(owner)
	setProp(owner, propZones, 1)
	if m.zoneBits(owner) != 1 {
		t.Fatal("the frame's own zone should take precedence")
	}
	removeProp(owner, propZones)
	removeProp(surface, propZones)
	if m.zoneBits(owner) != 0 {
		t.Fatal("outer frame stayed snapped after the video left its zone")
	}
	setProp(surface, propZones, 2)
	if m.zoneBits(owner) != 2 {
		t.Fatal("outer frame did not recognize the video returning to its zone")
	}
	r := windowRect(surface)
	r.Left += 100
	r.Right += 100
	if err := setWindowPos(surface, r, swpQuiet); err != nil {
		t.Fatal(err)
	}
	if m.zoneBits(owner) != 0 {
		t.Fatal("a detached surface kept its former frame snapped")
	}
}

func TestClippedOwnedContentKeepsItsZone(t *testing.T) {
	owner, surface := ownedTestWindows(t)
	m := newManager()
	w := m.track(owner)
	w.zoneSource = surface
	zone := visibleRect(owner)
	m.layouts[monitorFromWindow(owner)] = zoneLayout{zones: map[int]rect{2: zone}}
	setProp(surface, propZones, 4)
	m.placeClient(owner, w, zone)
	defer m.unhide(owner, w)
	// Real players reposition their video when the outer frame moves or resizes.
	r := clientScreenRect(owner)
	r.Left += 2
	r.Top += 32
	r.Right -= 2
	r.Bottom -= 2
	if err := setWindowPos(surface, r, swpQuiet); err != nil {
		t.Fatal(err)
	}
	if !w.hidden || m.zoneBits(owner) != 4 {
		t.Fatal("clipping the title bar made the current zone look stale")
	}
}

func TestOwnedDialogsStayIndependent(t *testing.T) {
	for _, mode := range []string{"framed", "tool", "modal", "small", "managed"} {
		t.Run(mode, func(t *testing.T) {
			_, surface := ownedTestWindows(t)
			switch mode {
			case "framed":
				if err := setStyles(surface, windowStyle(surface)|wsCaption, windowExStyle(surface)); err != nil {
					t.Fatal(err)
				}
			case "tool", "modal":
				ex := uint32(wsExToolWindow)
				if mode == "modal" {
					ex = wsExDlgModalFrame
				}
				if err := setStyles(surface, windowStyle(surface), windowExStyle(surface)|ex); err != nil {
					t.Fatal(err)
				}
			case "small":
				r := windowRect(surface)
				r.Left += 80
				r.Right -= 80
				if err := setWindowPos(surface, r, swpQuiet); err != nil {
					t.Fatal(err)
				}
			case "managed":
				setProp(surface, propHidden, 1)
			}
			if got := frameWindow(surface); got != surface {
				t.Fatalf("independent %s window redirected to %d", mode, got)
			}
		})
	}
}
