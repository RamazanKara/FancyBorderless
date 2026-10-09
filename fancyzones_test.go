package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCustomLayoutFor(t *testing.T) {
	mon := monitor{pnp: "TEST", instance: "INSTANCE", work: rect{-1000, -800, 0, 0}, dpi: 96}
	for _, tc := range []struct {
		name, kind, info string
		want             map[int]rect
		wantError        bool
	}{
		{"grid", "grid", `{"rows":1,"columns":2,"rows-percentage":[10000],"columns-percentage":[5000,5000],"cell-child-map":[[0,1]]}`,
			map[int]rect{0: {-990, -790, -505, -10}, 1: {-495, -790, -10, -10}}, false},
		{"canvas ignores grid spacing", "canvas", `{"ref-width":1000,"ref-height":800,"zones":[{"X":10,"Y":20,"width":400,"height":300}]}`,
			map[int]rect{0: {-990, -780, -590, -480}}, false},
		{"invalid JSON", "grid", `{`, nil, true},
		{"invalid grid", "grid", `{}`, nil, true},
		{"invalid canvas", "canvas", `{"ref-width":1000}`, nil, true},
		{"unknown type", "other", `{}`, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entry := fzApplied{}
			entry.Device.Monitor, entry.Device.Instance = "test", "instance"
			entry.Layout.Type, entry.Layout.UUID = "custom", "{abc}"
			entry.Layout.ShowSpacing, entry.Layout.Spacing = true, 10
			f := &fancyZones{
				applied: []fzApplied{entry},
				custom:  map[string]fzCustomLayout{"{ABC}": {Name: tc.name, Type: tc.kind, Info: json.RawMessage(tc.info)}},
			}
			got := f.layoutFor(mon, "")
			if (got.err != nil) != tc.wantError || got.name != tc.name || !reflect.DeepEqual(got.zones, tc.want) {
				t.Fatalf("layout = %+v; want zones %v, error %v", got, tc.want, tc.wantError)
			}
		})
	}
	f := &fancyZones{applied: []fzApplied{{}}}
	f.applied[0].Device.Monitor = mon.pnp
	f.applied[0].Layout.Type = "custom"
	if got := f.layoutFor(mon, ""); got.err == nil {
		t.Fatal("missing custom layout was accepted")
	}
	f.applied[0].Layout.Type = "unknown"
	f.applied[0].Layout.ZoneCount = 1
	if got := f.layoutFor(mon, ""); got.err == nil {
		t.Fatal("unknown template was accepted")
	}
	f.applied = nil
	if got := f.layoutFor(mon, ""); got.err == nil {
		t.Fatal("missing monitor layout was accepted")
	}
}

func TestCustomLayoutReloadKeepsPreviousLayoutsOnError(t *testing.T) {
	f := &fancyZones{dir: t.TempDir()}
	paths := f.files()
	if err := os.WriteFile(paths[0], []byte(`{"applied-layouts":[{"applied-layout":{"type":"columns","zone-count":2}}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths[1], []byte(`{"custom-layouts":[{"uuid":"{abc}","name":"Saved","type":"grid"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := f.load(); err != nil {
		t.Fatal(err)
	}
	if f.custom["{ABC}"].Name != "Saved" {
		t.Fatal("custom layout UUID was not normalized")
	}
	if err := os.WriteFile(filepath.Join(f.dir, "custom-layouts.json"), []byte(`{"custom-layouts":`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := f.load(); err == nil {
		t.Fatal("partial custom layout write was accepted")
	}
	if len(f.applied) != 1 || f.custom["{ABC}"].Name != "Saved" {
		t.Fatal("failed reload discarded the last valid layouts")
	}
}
