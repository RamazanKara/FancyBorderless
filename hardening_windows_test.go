package main

import (
	"runtime"
	"testing"
	"time"
)

func managedTestWindow(t *testing.T, clipped bool) (*manager, uintptr, *window, rect) {
	t.Helper()
	runtime.LockOSThread()
	t.Cleanup(runtime.UnlockOSThread)
	procSetProcessDpiAwarenessContext.Call(dpiAwarenessPerMonitorV2)
	h := frameTestWindow(t, "standard")
	m := newManager()
	m.cfg = defaultConfig()
	w := m.track(h)
	w.offset = clipped
	// These tests drive evaluation directly, without installing event hooks.
	m.moveHooks[w.pid] = 0
	zone := visibleRect(h)
	m.layouts[monitorFromWindow(h)] = zoneLayout{zones: map[int]rect{0: zone}}
	setProp(h, propZones, 1)
	m.handleSnapped(h, w, 1)
	if !w.stripped && !w.hidden {
		t.Fatal("fixture did not remove or hide the frame")
	}
	return m, h, w, zone
}

func TestMissingZoneRestoresFrame(t *testing.T) {
	for _, clipped := range []bool{false, true} {
		m, h, w, _ := managedTestWindow(t, clipped)
		m.layouts[monitorFromWindow(h)] = zoneLayout{zones: map[int]rect{}}
		m.handleSnapped(h, w, 1)
		if w.stripped || w.hidden || windowStyle(h)&wsCaption != wsCaption || hasRegion(h) {
			t.Errorf("clipped=%v: removing the zone left the frame hidden", clipped)
		}
	}
}

func TestFullscreenSuspendsZoneManagement(t *testing.T) {
	for _, clipped := range []bool{false, true} {
		m, h, w, zone := managedTestWindow(t, clipped)
		mi, ok := monitorInfo(monitorFromWindow(h))
		if !ok {
			t.Fatal("test monitor not found")
		}
		if err := setStyles(h, (windowStyle(h)&^frameStyles)|wsPopup, windowExStyle(h)&^frameExStyles); err != nil {
			t.Fatal(err)
		}
		if err := setWindowPos(h, mi.Monitor, swpQuiet|swpFrameChanged); err != nil {
			t.Fatal(err)
		}
		m.handleSnapped(h, w, 1)
		if windowRect(h) != mi.Monitor || hasRegion(h) || windowStyle(h)&frameStyles != 0 {
			t.Errorf("clipped=%v: fullscreen was resized, clipped or reframed", clipped)
		}
		removeProp(h, propZones)
		m.showTitleBar(h, w, "test unsnap during fullscreen")
		if windowRect(h) != mi.Monitor || windowStyle(h)&frameStyles != 0 {
			t.Errorf("clipped=%v: restoring borders changed app-owned fullscreen", clipped)
		}
		if err := setStyles(h, wsCaption|wsThickFrame, windowExStyle(h)); err != nil {
			t.Fatal(err)
		}
		if err := setVisibleRect(h, zone); err != nil {
			t.Fatal(err)
		}
		setProp(h, propZones, 1)
		m.handleSnapped(h, w, 1)
		if !w.stripped && !w.hidden {
			t.Errorf("clipped=%v: management did not resume after fullscreen", clipped)
		}
	}
}

func TestMovedWindowRejectsStaleZoneMarker(t *testing.T) {
	for _, clipped := range []bool{false, true} {
		m, h, w, zone := managedTestWindow(t, clipped)
		r := windowRect(h)
		r.Left += 100
		r.Right += 100
		if err := setWindowPos(h, r, swpQuiet); err != nil {
			t.Fatal(err)
		}
		if m.zoneBits(h) != 0 {
			t.Errorf("clipped=%v: stale marker still counts as snapped", clipped)
		}
		m.showTitleBar(h, w, "test move off zone")
		if w.stripped || w.hidden || windowStyle(h)&wsCaption != wsCaption {
			t.Errorf("clipped=%v: moving off zone did not restore the frame", clipped)
		}
		if m.zoneBits(h) != 0 {
			t.Errorf("clipped=%v: restoring the frame reaccepted the stale marker", clipped)
		}
		if err := setVisibleRect(h, zone); err != nil {
			t.Fatal(err)
		}
		if m.zoneBits(h) != 1 {
			t.Errorf("clipped=%v: snapping back to the same zone was not recognized", clipped)
		}
	}
}

func TestSnapEndsManualOverride(t *testing.T) {
	m, h, w, _ := managedTestWindow(t, false)
	w.manual = true
	m.handleSnapped(h, w, 1)
	if w.manual {
		t.Fatal("snapping kept the manual override that prevents unsnap restoration")
	}
}

func TestDeadWindowEvaluationDropsOriginalHandle(t *testing.T) {
	h, _ := ownedTestWindows(t)
	m := newManager()
	m.track(h)
	procDestroyWindow.Call(h)
	m.evaluate(h)
	if len(m.windows) != 0 {
		t.Fatal("evaluation retained state under a destroyed window handle")
	}
}

func TestReusedHandleDropsPreviousProcess(t *testing.T) {
	h, _ := ownedTestWindows(t)
	m := newManager()
	w := m.track(h)
	w.pid++
	m.evaluate(h)
	if m.windows[h] == w {
		t.Fatal("evaluation retained another process's window state")
	}
}

func TestDestroyEventDropsWindowAndOwnedSource(t *testing.T) {
	h, source := ownedTestWindows(t)
	m := newManager()
	m.hwnd = h
	w := m.track(h)
	w.zoneSource = source
	m.track(source)
	m.onEvent(eventObjectDestroy, source)
	if m.windows[source] != nil || w.zoneSource != 0 {
		t.Fatal("destroyed content window retained tracking or an inherited marker")
	}
	m.onEvent(eventObjectDestroy, h)
	if len(m.windows) != 0 {
		t.Fatal("destroyed frame retained its saved state")
	}
}

func TestNewGameResolutionReplacesSavedBounds(t *testing.T) {
	m, h, w, zone := managedTestWindow(t, false)
	w.fixedSize = true
	w.strippedAt = time.Now().Add(-refusalWindow - time.Second)
	if err := setStyles(h, wsCaption, 0); err != nil {
		t.Fatal(err)
	}
	want := rect{zone.Left, zone.Top, zone.Left + 640, zone.Top + 480}
	if err := setWindowPos(h, want, swpQuiet|swpFrameChanged); err != nil {
		t.Fatal(err)
	}
	m.handleSnapped(h, w, 1)
	removeProp(h, propZones)
	m.showTitleBar(h, w, "test resolution change")
	if got := windowRect(h); got.width() != want.width() || got.height() != want.height() {
		t.Fatalf("restored %v, want new game size %v", got, want)
	}
}

func TestFailedWindowChangeDoesNotRetry(t *testing.T) {
	m := newManager()
	w := &window{titleBar: true}
	m.strip(0, w, rect{0, 0, 800, 600})
	if !w.broken || w.stripped {
		t.Fatal("failed style change was recorded as successful")
	}
	changes := len(w.changes)
	m.handleSnapped(0, w, 1)
	if len(w.changes) != changes || w.hidden {
		t.Fatal("a refused change was retried")
	}
}

func TestDPIChangeRemeasuresClippedTitleBar(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	procSetProcessDpiAwarenessContext.Call(dpiAwarenessPerMonitorV2)
	h := frameTestWindow(t, "custom_hittest")
	m := newManager()
	w := m.track(h)
	want := w.ownBar
	if want == 0 {
		t.Fatal("fixture has no custom title bar")
	}
	m.placeClient(h, w, contentRect(h, w))
	w.bits, w.dpi, w.ownBar, w.gaveUp = 1, w.dpi*2, want*2, true
	m.updateDPI(h, w)
	if w.ownBar != want || w.gaveUp || !w.refit {
		t.Fatalf("DPI change kept old measurements or change budget: %+v", w)
	}
	m.unhide(h, w)
}

func TestLayoutAndDisplayChangesRefitSnappedWindow(t *testing.T) {
	for _, clipped := range []bool{false, true} {
		m, h, w, zone := managedTestWindow(t, clipped)
		control, _ := ownedTestWindows(t)
		m.hwnd = control
		w.bits = 1
		zone.Left += 150
		zone.Right += 350
		m.layouts[monitorFromWindow(h)] = zoneLayout{zones: map[int]rect{0: zone}}
		m.refitAll()
		procKillTimer.Call(control, h)
		if m.zoneBits(h) != 1 {
			t.Fatal("layout change detached a snapped window before it could be refitted")
		}
		m.handleSnapped(h, w, 1)
		bounds := windowRect(h)
		if clipped {
			bounds = contentRect(h, w)
		}
		if bounds != zone {
			t.Errorf("clipped=%v: refitted to %v, want %v", clipped, bounds, zone)
		}
		m.onTimer(timerRefit)
		procKillTimer.Call(control, h)
		if len(m.layouts) != 0 || !w.refit {
			t.Fatal("display change kept stale zone geometry")
		}
	}
}
