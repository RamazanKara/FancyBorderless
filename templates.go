package main

import "fmt"

// The built-in FancyZones templates aren't stored as zones; FancyZones computes them from the
// zone count and spacing. These functions port LayoutConfigurator.cpp from PowerToys,
// including its integer truncation, so the rectangles match FancyZones' own to the pixel.

func templateZones(kind string, work rect, count int, spacing int64) (map[int]rect, error) {
	if count <= 0 || kind == "blank" {
		return map[int]rect{}, nil
	}
	switch kind {
	case "focus":
		return focusZones(work, count), nil
	case "columns":
		return columnZones(work, count, spacing), nil
	case "rows":
		return rowZones(work, count, spacing), nil
	case "grid":
		return gridZones(work, gridTemplate(count), spacing)
	case "priority-grid":
		if count < len(priorityGrids) {
			return gridZones(work, priorityGrids[count-1], spacing)
		}
		return gridZones(work, gridTemplate(count), spacing)
	}
	return nil, fmt.Errorf("unknown FancyZones template %q", kind)
}

func offset(work rect, left, top, right, bottom int64) rect {
	return rect{work.Left + int32(left), work.Top + int32(top), work.Left + int32(right), work.Top + int32(bottom)}
}

func focusZones(work rect, count int) map[int]rect {
	var left, top int64 = 100, 100
	right := left + int64(float64(work.width())*0.4)
	bottom := top + int64(float64(work.height())*0.4)
	var step int64
	if count > 1 {
		step = 50
	}
	zones := map[int]rect{}
	for i := 0; i < count; i++ {
		zones[i] = offset(work, left, top, right, bottom)
		left, right, top, bottom = left+step, right+step, top+step, bottom+step
	}
	return zones
}

func columnZones(work rect, count int, spacing int64) map[int]rect {
	n := int64(count)
	totalWidth := int64(work.width()) - spacing*(n+1)
	totalHeight := int64(work.height()) - spacing*2
	left, top := spacing, spacing
	zones := map[int]rect{}
	for i := int64(0); i < n; i++ {
		right := left + (i+1)*totalWidth/n - i*totalWidth/n
		zones[int(i)] = offset(work, left, top, right, totalHeight+spacing)
		left = right + spacing
	}
	return zones
}

func rowZones(work rect, count int, spacing int64) map[int]rect {
	n := int64(count)
	totalWidth := int64(work.width()) - spacing*2
	totalHeight := int64(work.height()) - spacing*(n+1)
	left, top := spacing, spacing
	zones := map[int]rect{}
	for i := int64(0); i < n; i++ {
		bottom := top + (i+1)*totalHeight/n - i*totalHeight/n
		zones[int(i)] = offset(work, left, top, totalWidth+spacing, bottom)
		top = bottom + spacing
	}
	return zones
}

// gridTemplate builds the "Grid" template: as many rows as fit a square-ish grid, the last
// zone stretching over any cells left over.
func gridTemplate(count int) fzGrid {
	rows := 1
	for count/rows >= rows {
		rows++
	}
	rows--
	cols := count / rows
	if count%rows != 0 {
		cols++
	}
	g := fzGrid{Rows: rows, Columns: cols}
	for r := 0; r < rows; r++ {
		g.RowsPercentage = append(g.RowsPercentage, int64(10000*(r+1)/rows-10000*r/rows))
	}
	for c := 0; c < cols; c++ {
		g.ColumnsPercentage = append(g.ColumnsPercentage, int64(10000*(c+1)/cols-10000*c/cols))
	}
	index := 0
	for r := 0; r < rows; r++ {
		row := make([]int, cols)
		for c := range row {
			row[c] = index
			index++
			if index == count {
				index--
			}
		}
		g.CellChildMap = append(g.CellChildMap, row)
	}
	return g
}

// priorityGrids are the predefined "Priority Grid" templates for 1 to 11 zones; FancyZones
// uses them for fewer than 11 zones and the plain grid otherwise.
var priorityGrids = []fzGrid{
	{1, 1, []int64{10000}, []int64{10000}, [][]int{{0}}},
	{1, 2, []int64{10000}, []int64{6667, 3333}, [][]int{{0, 1}}},
	{1, 3, []int64{10000}, []int64{2500, 5000, 2500}, [][]int{{0, 1, 2}}},
	{2, 3, []int64{5000, 5000}, []int64{2500, 5000, 2500}, [][]int{{0, 1, 2}, {0, 1, 3}}},
	{2, 3, []int64{5000, 5000}, []int64{2500, 5000, 2500}, [][]int{{0, 1, 2}, {3, 1, 4}}},
	{3, 3, []int64{3333, 3334, 3333}, []int64{2500, 5000, 2500}, [][]int{{0, 1, 2}, {0, 1, 3}, {4, 1, 5}}},
	{3, 3, []int64{3333, 3334, 3333}, []int64{2500, 5000, 2500}, [][]int{{0, 1, 2}, {3, 1, 4}, {5, 1, 6}}},
	{3, 4, []int64{3333, 3334, 3333}, []int64{2500, 2500, 2500, 2500}, [][]int{{0, 1, 2, 3}, {4, 1, 2, 5}, {6, 1, 2, 7}}},
	{3, 4, []int64{3333, 3334, 3333}, []int64{2500, 2500, 2500, 2500}, [][]int{{0, 1, 2, 3}, {4, 1, 2, 5}, {6, 1, 7, 8}}},
	{3, 4, []int64{3333, 3334, 3333}, []int64{2500, 2500, 2500, 2500}, [][]int{{0, 1, 2, 3}, {4, 1, 5, 6}, {7, 1, 8, 9}}},
	{3, 4, []int64{3333, 3334, 3333}, []int64{2500, 2500, 2500, 2500}, [][]int{{0, 1, 2, 3}, {4, 1, 5, 6}, {7, 8, 9, 10}}},
}
