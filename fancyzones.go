package main

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

type fzApplied struct {
	Device struct {
		Monitor  string `json:"monitor"`
		Instance string `json:"monitor-instance"`
		Desktop  string `json:"virtual-desktop"`
	} `json:"device"`
	Layout struct {
		UUID        string `json:"uuid"`
		Type        string `json:"type"`
		ShowSpacing bool   `json:"show-spacing"`
		Spacing     int64  `json:"spacing"`
		ZoneCount   int    `json:"zone-count"`
	} `json:"applied-layout"`
}

type fzCustomLayout struct {
	UUID string          `json:"uuid"`
	Name string          `json:"name"`
	Type string          `json:"type"`
	Info json.RawMessage `json:"info"`
}

type fzGrid struct {
	Rows              int     `json:"rows"`
	Columns           int     `json:"columns"`
	RowsPercentage    []int64 `json:"rows-percentage"`
	ColumnsPercentage []int64 `json:"columns-percentage"`
	CellChildMap      [][]int `json:"cell-child-map"`
}

type fzCanvas struct {
	RefWidth  int `json:"ref-width"`
	RefHeight int `json:"ref-height"`
	Zones     []struct {
		X      int `json:"X"`
		Y      int `json:"Y"`
		Width  int `json:"width"`
		Height int `json:"height"`
	} `json:"zones"`
}

// fancyZones reads the layout files in FancyZones' settings folder. They are the only
// outside record of which layout is applied where, and FancyZones rewrites them whenever a
// layout is applied.
type fancyZones struct {
	dir     string
	applied []fzApplied
	custom  map[string]fzCustomLayout
}

func newFancyZones() *fancyZones {
	return &fancyZones{dir: filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "PowerToys", "FancyZones")}
}

func (f *fancyZones) files() []string {
	return []string{
		filepath.Join(f.dir, "applied-layouts.json"),
		filepath.Join(f.dir, "custom-layouts.json"),
	}
}

func (f *fancyZones) load() error {
	var applied struct {
		Layouts []fzApplied `json:"applied-layouts"`
	}
	var custom struct {
		Layouts []fzCustomLayout `json:"custom-layouts"`
	}
	paths := f.files()
	if err := readJSON(paths[0], &applied); err != nil {
		return err
	}
	if err := readJSON(paths[1], &custom); err != nil {
		return err
	}
	f.applied = applied.Layouts
	f.custom = make(map[string]fzCustomLayout, len(custom.Layouts))
	for _, l := range custom.Layouts {
		f.custom[strings.ToUpper(l.UUID)] = l
	}
	return nil
}

type monitor struct {
	device   string // \\.\DISPLAY1
	pnp      string // SAM7454
	instance string // 7&17842d4&0&UID516
	bounds   rect
	work     rect
	primary  bool
}

// identifyMonitor derives the ids FancyZones keys applied-layouts.json with from the
// monitor's device interface path, e.g. \\?\DISPLAY#SAM7454#7&17842d4&0&UID516#{e6f07b5f-...}.
func identifyMonitor(hmon uintptr) (monitor, bool) {
	mi, ok := monitorInfo(hmon)
	if !ok {
		return monitor{}, false
	}
	m := monitor{
		device:  syscall.UTF16ToString(mi.Device[:]),
		bounds:  mi.Monitor,
		work:    mi.Work,
		primary: mi.Flags&monitorInfoPrimary != 0,
	}
	for i := uintptr(0); ; i++ {
		dd := displayDevice{}
		dd.Size = uint32(unsafe.Sizeof(dd))
		r, _, _ := procEnumDisplayDevicesW.Call(uintptr(unsafe.Pointer(&mi.Device[0])), i, uintptr(unsafe.Pointer(&dd)), eddGetDeviceInterfaceName)
		if r == 0 {
			break
		}
		if dd.StateFlags&displayDeviceActive == 0 {
			continue
		}
		parts := strings.Split(syscall.UTF16ToString(dd.DeviceID[:]), "#")
		if len(parts) >= 3 {
			m.pnp, m.instance = parts[1], parts[2]
			break
		}
	}
	return m, true
}

type zoneLayout struct {
	name  string
	zones map[int]rect // zone index -> screen rectangle
	err   error
}

func (f *fancyZones) layoutFor(m monitor, desktop string) zoneLayout {
	entry := f.appliedFor(m, desktop)
	if entry == nil {
		return zoneLayout{err: fmt.Errorf("FancyZones has no layout for monitor %s (%s)", m.pnp, m.device)}
	}
	var spacing int64
	if entry.Layout.ShowSpacing {
		spacing = entry.Layout.Spacing
	}
	if entry.Layout.Type != "custom" {
		zones, err := templateZones(entry.Layout.Type, m.work, entry.Layout.ZoneCount, spacing)
		if err != nil {
			return zoneLayout{name: entry.Layout.Type, err: err}
		}
		return zoneLayout{name: fmt.Sprintf("%s template, %d zones", entry.Layout.Type, entry.Layout.ZoneCount), zones: zones}
	}
	custom, ok := f.custom[strings.ToUpper(entry.Layout.UUID)]
	if !ok {
		return zoneLayout{err: fmt.Errorf("custom layout %s is missing from custom-layouts.json", entry.Layout.UUID)}
	}
	var zones map[int]rect
	var err error
	switch custom.Type {
	case "grid":
		var g fzGrid
		if err = json.Unmarshal(custom.Info, &g); err == nil {
			zones, err = gridZones(m.work, g, spacing)
		}
	case "canvas":
		var c fzCanvas
		if err = json.Unmarshal(custom.Info, &c); err == nil {
			zones, err = canvasZones(m.work, c)
		}
	default:
		err = fmt.Errorf("unknown layout type %q", custom.Type)
	}
	if err != nil {
		return zoneLayout{name: custom.Name, err: fmt.Errorf("layout %q: %w", custom.Name, err)}
	}
	return zoneLayout{name: custom.Name, zones: zones}
}

// appliedFor picks the applied-layouts entry for a monitor. The PnP id must match; the
// device instance and the virtual desktop decide between entries for the same model.
func (f *fancyZones) appliedFor(m monitor, desktop string) *fzApplied {
	var best *fzApplied
	bestScore := -1
	for i := range f.applied {
		e := &f.applied[i]
		if m.pnp == "" || !strings.EqualFold(e.Device.Monitor, m.pnp) {
			continue
		}
		score := 0
		if strings.EqualFold(e.Device.Instance, m.instance) {
			score += 2
		}
		if desktop != "" && strings.EqualFold(e.Device.Desktop, desktop) {
			score++
		}
		if score > bestScore {
			best, bestScore = e, score
		}
	}
	return best
}

// gridZones is a port of CalculateGridZones in PowerToys' LayoutConfigurator.cpp, including
// its integer truncation, so the rectangles match FancyZones' own to the pixel.
func gridZones(work rect, g fzGrid, spacing int64) (map[int]rect, error) {
	if g.Rows <= 0 || g.Columns <= 0 || len(g.RowsPercentage) != g.Rows ||
		len(g.ColumnsPercentage) != g.Columns || len(g.CellChildMap) != g.Rows {
		return nil, errors.New("malformed grid")
	}
	for _, row := range g.CellChildMap {
		if len(row) != g.Columns {
			return nil, errors.New("malformed grid")
		}
	}
	type span struct{ start, end int64 }
	split := func(total int64, percents []int64) []span {
		spans := make([]span, len(percents))
		var sum int64
		for i, p := range percents {
			spans[i].start = sum * total / 10000
			sum += p
			spans[i].end = sum * total / 10000
		}
		return spans
	}
	rows := split(int64(work.height()), g.RowsPercentage)
	cols := split(int64(work.width()), g.ColumnsPercentage)
	cells := g.CellChildMap

	zones := map[int]rect{}
	for r := 0; r < g.Rows; r++ {
		for c := 0; c < g.Columns; c++ {
			id := cells[r][c]
			if (r > 0 && cells[r-1][c] == id) || (c > 0 && cells[r][c-1] == id) {
				continue // not the top-left cell of this zone
			}
			maxR, maxC := r, c
			for maxR+1 < g.Rows && cells[maxR+1][c] == id {
				maxR++
			}
			for maxC+1 < g.Columns && cells[r][maxC+1] == id {
				maxC++
			}
			left, top := cols[c].start, rows[r].start
			right, bottom := cols[maxC].end, rows[maxR].end
			if r == 0 {
				top += spacing
			} else {
				top += spacing / 2
			}
			if maxR == g.Rows-1 {
				bottom -= spacing
			} else {
				bottom -= spacing / 2
			}
			if c == 0 {
				left += spacing
			} else {
				left += spacing / 2
			}
			if maxC == g.Columns-1 {
				right -= spacing
			} else {
				right -= spacing / 2
			}
			zones[id] = rect{
				Left:   work.Left + int32(left),
				Top:    work.Top + int32(top),
				Right:  work.Left + int32(right),
				Bottom: work.Top + int32(bottom),
			}
		}
	}
	return zones, nil
}

// canvasZones scales the editor's zone rectangles from the work area they were drawn on to
// the current one, like LayoutConfigurator::Custom (the DPI conversions there cancel out).
func canvasZones(work rect, c fzCanvas) (map[int]rect, error) {
	if c.RefWidth <= 0 || c.RefHeight <= 0 {
		return nil, errors.New("malformed canvas")
	}
	sx := float32(work.width()) / float32(c.RefWidth)
	sy := float32(work.height()) / float32(c.RefHeight)
	zones := map[int]rect{}
	for i, z := range c.Zones {
		x, y := float32(z.X)*sx, float32(z.Y)*sy
		w, h := float32(z.Width)*sx, float32(z.Height)*sy
		zones[i] = rect{
			Left:   work.Left + int32(x),
			Top:    work.Top + int32(y),
			Right:  work.Left + int32(x+w),
			Bottom: work.Top + int32(y+h),
		}
	}
	return zones, nil
}

// currentDesktop returns the active virtual desktop id in the format FancyZones stores, or
// "" when Windows doesn't expose it.
func currentDesktop() string {
	var key syscall.Handle
	path := utf16Ptr(`Software\Microsoft\Windows\CurrentVersion\Explorer\VirtualDesktops`)
	if syscall.RegOpenKeyEx(syscall.HKEY_CURRENT_USER, path, 0, syscall.KEY_READ, &key) != nil {
		return ""
	}
	defer syscall.RegCloseKey(key)
	var b [16]byte
	n := uint32(len(b))
	if syscall.RegQueryValueEx(key, utf16Ptr("CurrentVirtualDesktop"), nil, nil, &b[0], &n) != nil || n != 16 {
		return ""
	}
	return fmt.Sprintf("{%08X-%04X-%04X-%X-%X}",
		binary.LittleEndian.Uint32(b[0:4]), binary.LittleEndian.Uint16(b[4:6]),
		binary.LittleEndian.Uint16(b[6:8]), b[8:10], b[10:16])
}
