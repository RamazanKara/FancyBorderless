package main

import (
	"reflect"
	"testing"
)

func TestTemplateZones(t *testing.T) {
	for _, tc := range []struct {
		name, kind string
		work       rect
		count      int
		spacing    int64
		want       map[int]rect
	}{
		{"blank", "blank", rect{0, 0, 1000, 800}, 3, 0, map[int]rect{}},
		{"zero zones", "grid", rect{0, 0, 1000, 800}, 0, 0, map[int]rect{}},
		{"negative count", "grid", rect{0, 0, 1000, 800}, -1, 0, map[int]rect{}},
		{"single focus", "focus", rect{-1000, -800, 0, 0}, 1, 20,
			map[int]rect{0: {-900, -700, -500, -380}}},
		{"overlapping focus", "focus", rect{-1000, -800, 0, 0}, 2, 0,
			map[int]rect{0: {-900, -700, -500, -380}, 1: {-850, -650, -450, -330}}},
		{"columns with spacing", "columns", rect{-100, -200, 901, 801}, 2, 3,
			map[int]rect{0: {-97, -197, 399, 798}, 1: {402, -197, 898, 798}}},
		{"rows with spacing", "rows", rect{-100, -200, 901, 801}, 2, 3,
			map[int]rect{0: {-97, -197, 898, 299}, 1: {-97, 302, 898, 798}}},
		{"column remainder", "columns", rect{0, 0, 1000, 800}, 3, 0,
			map[int]rect{0: {0, 0, 333, 800}, 1: {333, 0, 666, 800}, 2: {666, 0, 1000, 800}}},
		{"row remainder", "rows", rect{0, 0, 800, 1000}, 3, 0,
			map[int]rect{0: {0, 0, 800, 333}, 1: {0, 333, 800, 666}, 2: {0, 666, 800, 1000}}},
		{"single grid", "grid", rect{0, 0, 1000, 800}, 1, 10,
			map[int]rect{0: {10, 10, 990, 790}}},
		{"merged final grid cell", "grid", rect{0, 0, 1200, 800}, 5, 0,
			map[int]rect{0: {0, 0, 399, 400}, 1: {399, 0, 799, 400}, 2: {799, 0, 1200, 400}, 3: {0, 400, 399, 800}, 4: {399, 400, 1200, 800}}},
		{"priority widths", "priority-grid", rect{0, 0, 1200, 800}, 2, 5,
			map[int]rect{0: {5, 5, 798, 795}, 1: {802, 5, 1195, 795}}},
		{"priority switches to grid at eleven", "priority-grid", rect{0, 0, 1200, 900}, 11, 0,
			map[int]rect{
				0: {0, 0, 300, 299}, 1: {300, 0, 600, 299}, 2: {600, 0, 900, 299}, 3: {900, 0, 1200, 299},
				4: {0, 299, 300, 599}, 5: {300, 299, 600, 599}, 6: {600, 299, 900, 599}, 7: {900, 299, 1200, 599},
				8: {0, 599, 300, 900}, 9: {300, 599, 600, 900}, 10: {600, 599, 1200, 900},
			}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := templateZones(tc.kind, tc.work, tc.count, tc.spacing)
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("zones = %v, %v; want %v", got, err, tc.want)
			}
		})
	}
	if _, err := templateZones("unknown", rect{0, 0, 1000, 800}, 1, 0); err == nil {
		t.Fatal("unknown template was accepted")
	}
}

func TestGridZones(t *testing.T) {
	g := fzGrid{2, 2, []int64{5000, 5000}, []int64{5000, 5000}, [][]int{{0, 1}, {0, 2}}}
	got, err := gridZones(rect{-1000, -800, 0, 0}, g, 5)
	want := map[int]rect{0: {-995, -795, -502, -5}, 1: {-498, -795, -5, -402}, 2: {-498, -398, -5, -5}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("merged grid = %v, %v; want %v", got, err, want)
	}
	for _, tc := range []struct {
		name string
		grid fzGrid
	}{
		{"empty", fzGrid{}},
		{"missing percentages", fzGrid{Rows: 1, Columns: 1, CellChildMap: [][]int{{0}}}},
		{"missing row", fzGrid{1, 1, []int64{10000}, []int64{10000}, nil}},
		{"ragged row", fzGrid{1, 1, []int64{10000}, []int64{10000}, [][]int{{0, 1}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := gridZones(rect{0, 0, 1000, 800}, tc.grid, 0); err == nil {
				t.Fatal("malformed grid was accepted")
			}
		})
	}
}
