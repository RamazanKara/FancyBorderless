package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCanvasRoundingAtDifferentDPIs(t *testing.T) {
	var canvas fzCanvas
	if err := json.Unmarshal([]byte(`{"ref-width":1280,"ref-height":720,"zones":[{"X":14,"Y":10,"width":640,"height":360}]}`), &canvas); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		dpi  uint32
		left int32
	}{{96, -1899}, {120, -1900}, {144, -1899}, {168, -1899}, {192, -1899}} {
		zones, err := canvasZones(rect{-1920, -1080, 0, 0}, canvas, tc.dpi)
		// At 125%, PowerToys truncates 20.999998 to 20 for the left edge.
		want := rect{tc.left, -1065, -939, -525}
		if err != nil || zones[0] != want {
			t.Errorf("DPI %d: got %v (%v), want %v", tc.dpi, zones[0], err, want)
		}
	}
}

func TestFullscreenDetection(t *testing.T) {
	screen := rect{-2560, -200, 0, 1240}
	zone := rect{-2560, -200, -1280, 1240}
	for _, tc := range []struct {
		name           string
		style          uint32
		bounds, placed rect
		stripped, want bool
	}{
		{"app fullscreen", wsPopup, screen, zone, true, true},
		{"fullscreen without popup", 0, screen, zone, true, true},
		{"our full monitor zone", 0, screen, screen, true, false},
		{"app changed full monitor zone to popup", wsPopup, screen, screen, true, true},
		{"ordinary stripped zone", 0, zone, zone, true, false},
		{"framed monitor sized window", wsCaption, screen, zone, false, false},
		{"new fullscreen window", wsPopup, screen, rect{}, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := appFullscreen(tc.style, tc.bounds, screen, tc.placed, wsCaption|wsThickFrame, tc.stripped); got != tc.want {
				t.Fatalf("fullscreen=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestZoneCornerAcrossMonitorOrigins(t *testing.T) {
	zone := rect{-2560, -1440, -1280, 0}
	for _, tc := range []struct {
		bounds    rect
		tolerance int32
		want      bool
	}{
		{zone, 8, true},
		{rect{-2560, -1440, -1920, -960}, 8, true},
		{rect{-2550, -1430, -1270, 10}, 12, true},
		{rect{-2550, -1430, -1270, 10}, 8, false},
		{rect{-2460, -1440, -1180, 0}, 16, false},
		{rect{0, 0, 1280, 1440}, 16, false},
	} {
		if got := atZone(tc.bounds, zone, tc.tolerance); got != tc.want {
			t.Errorf("%v: atZone=%v, want %v", tc.bounds, got, tc.want)
		}
	}
}

func TestLayoutReloadAfterPartialWriteAndRestart(t *testing.T) {
	f := &fancyZones{dir: t.TempDir()}
	applied := `{"applied-layouts":[{"device":{"monitor":"TEST","monitor-instance":"SECOND","virtual-desktop":"DESKTOP"},"applied-layout":{"type":"columns","zone-count":2}}]}`
	path := filepath.Join(f.dir, "applied-layouts.json")
	write := func(data string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(applied)
	if err := f.load(); err != nil {
		t.Fatal(err)
	}
	mon := monitor{pnp: "TEST", instance: "SECOND", work: rect{-2560, 0, 0, 1440}}
	want := map[int]rect{0: {-2560, 0, -1280, 1440}, 1: {-1280, 0, 0, 1440}}
	for _, partial := range []string{"", `{"applied-layouts":`} {
		write(partial)
		if f.load() == nil {
			t.Fatal("partial write was accepted")
		}
		if got := f.layoutFor(mon, "DESKTOP"); got.err != nil || !reflect.DeepEqual(got.zones, want) {
			t.Fatalf("partial write discarded the last valid layout: %+v", got)
		}
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if f.load() == nil {
		t.Fatal("missing file was accepted")
	}
	write(applied)
	if err := f.load(); err != nil {
		t.Fatal(err)
	}
	mon.work = rect{0, -1080, 1920, 0}
	want = map[int]rect{0: {0, -1080, 960, 0}, 1: {960, -1080, 1920, 0}}
	if got := f.layoutFor(mon, "DESKTOP"); got.err != nil || !reflect.DeepEqual(got.zones, want) {
		t.Fatalf("restarted layout did not use the changed work area: %+v", got)
	}
}
