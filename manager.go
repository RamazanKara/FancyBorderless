package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

const (
	timerTick  = 1
	timerScan  = 2
	timerRefit = 3

	hotkeyToggleTitleBar = 1

	settleDelay = 150 * time.Millisecond

	// A program that undoes every change would otherwise be resized in a loop.
	changeBudget       = 6
	changeBudgetWindow = 30 * time.Second

	// A frame that comes back this soon after removing it was put back by the program.
	refusalWindow = 3 * time.Second
)

type window struct {
	exe string
	// titleBar: Windows draws a title bar above the window's content.
	titleBar bool
	// ownBar: height of a plain title bar the app draws itself (WPF and UWP apps, for example).
	// Browsers draw tabs there instead and get 0, so they're left alone.
	ownBar int32
	// fixedSize: the window had no resize border of its own. FancyZones never resizes such
	// windows and neither do we; games in windowed mode are usually like this, and most of
	// them keep drawing at their resolution, so a smaller window would cut the picture off.
	fixedSize   bool
	origStyle   uint32
	origExStyle uint32
	stripped    bool // title bar and border removed by us
	strippedAt  time.Time
	refusals    int  // times the program put its frame back right away
	offset      bool // title bar kept but hidden above the screen (enforced frame, or the app's own bar)
	squared     bool // rounded corners turned off by us
	manual      bool // title bar removed with the hotkey on a window that isn't snapped
	broken      bool // Windows refused a change, usually because the window runs elevated
	gaveUp      bool
	changes     []time.Time
}

type manager struct {
	hwnd     uintptr
	icon     uintptr
	ownPID   uint32
	cfg      config
	cfgStamp string
	fz       *fancyZones
	fzStamp  string
	desktop  string
	layouts  map[uintptr]zoneLayout // per monitor handle
	windows  map[uintptr]*window
	due      map[uintptr]time.Time
	reported map[string]bool
	hooks    []uintptr
}

func newManager() *manager {
	return &manager{
		ownPID:   uint32(os.Getpid()),
		fz:       newFancyZones(),
		layouts:  map[uintptr]zoneLayout{},
		windows:  map[uintptr]*window{},
		due:      map[uintptr]time.Time{},
		reported: map[string]bool{},
	}
}

// WINEVENT_OUTOFCONTEXT: Windows queues the events to this thread's message loop instead of
// loading code into the processes that raise them.
var winEventCallback = syscall.NewCallback(func(hook, event, hwnd, idObject, idChild, thread, eventTime uintptr) uintptr {
	if hwnd != 0 && int32(idObject) == objidWindow && int32(idChild) == childidSelf {
		app.onEvent(uint32(event), hwnd)
	}
	return 0
})

func (m *manager) hook() {
	ranges := [][2]uintptr{
		{eventSystemMoveSizeEnd, eventSystemMoveSizeEnd},
		{eventObjectDestroy, eventObjectShow},
		{eventObjectLocationChange, eventObjectLocationChange},
	}
	for _, r := range ranges {
		h, _, err := procSetWinEventHook.Call(r[0], r[1], 0, winEventCallback, 0, 0, winEventOutOfContext|winEventSkipOwnProcess)
		if h == 0 {
			log.Printf("SetWinEventHook %#x: %v", r[0], err)
			continue
		}
		m.hooks = append(m.hooks, h)
	}
}

func (m *manager) onEvent(event uint32, h uintptr) {
	switch event {
	case eventObjectDestroy:
		delete(m.windows, h)
		delete(m.due, h)
	case eventObjectLocationChange:
		// Raised for every moving window; only snapped or already handled ones matter.
		if _, ok := m.windows[h]; ok || zoneBits(h) != 0 {
			m.due[h] = time.Now().Add(settleDelay)
		}
	default:
		m.due[h] = time.Now().Add(settleDelay)
	}
}

func (m *manager) onTimer(id uintptr) {
	switch id {
	case timerTick:
		m.tick()
	case timerScan:
		m.scan()
	case timerRefit:
		procKillTimer.Call(m.hwnd, timerRefit)
		m.layouts = map[uintptr]zoneLayout{}
		m.refitAll()
	}
}

// scheduleRefit waits for a display or work area change to finish before refitting.
func (m *manager) scheduleRefit() {
	procSetTimer.Call(m.hwnd, timerRefit, 500, 0)
}

func (m *manager) tick() {
	if len(m.due) == 0 {
		return
	}
	// A layout switch moves windows and rewrites applied-layouts.json; fit against the new
	// layout, not the cached one.
	m.reloadIfChanged()
	now := time.Now()
	for h, t := range m.due {
		if now.Before(t) {
			continue
		}
		delete(m.due, h)
		m.evaluate(h)
	}
}

// scan catches what the events miss, such as FancyZones stamping a window after moving it.
func (m *manager) scan() {
	m.reloadIfChanged()
	for _, h := range topLevelWindows() {
		if _, ok := m.windows[h]; ok || zoneBits(h) != 0 {
			m.evaluate(h)
		}
	}
	for h := range m.windows {
		if !isWindow(h) {
			delete(m.windows, h)
		}
	}
}

func (m *manager) reloadIfChanged() {
	if fileStamp(configPath) != m.cfgStamp {
		m.reloadConfig()
	}
	if stamp := fileStamp(m.fz.files()...); stamp != m.fzStamp {
		if err := m.fz.load(); err != nil {
			// FancyZones may be halfway through writing; this is retried on the next pass.
			m.reportOnce("fz-load", "reading FancyZones layouts: %v", err)
			return
		}
		m.fzStamp = stamp
		delete(m.reported, "fz-load")
		log.Print("FancyZones layouts loaded")
		m.layouts = map[uintptr]zoneLayout{}
		m.refitAll()
	}
	if d := currentDesktop(); d != m.desktop {
		m.desktop = d
		m.layouts = map[uintptr]zoneLayout{}
		m.refitAll()
	}
}

func (m *manager) reloadConfig() {
	cfg, err := loadConfig()
	if err != nil {
		log.Printf("settings: %v; using the defaults", err)
		beep()
	}
	old := m.cfg
	m.cfg = cfg
	m.cfgStamp = fileStamp(configPath)
	m.registerHotkeys()
	if old.SquareCorners != cfg.SquareCorners {
		m.applyCorners()
	}
	if old.RemoveTitleBars && !cfg.RemoveTitleBars {
		m.restoreAll("turned off")
	}
	m.refitAll()
	m.updateTrayIcon()
	log.Print("settings loaded")
}

func (m *manager) saveConfig() {
	if err := writeJSON(configPath, m.cfg); err != nil {
		log.Printf("saving settings: %v", err)
		return
	}
	m.cfgStamp = fileStamp(configPath)
}

// refitAll re-evaluates every handled window with a fresh change budget.
func (m *manager) refitAll() {
	now := time.Now()
	for h, w := range m.windows {
		w.changes, w.gaveUp = nil, false
		m.due[h] = now
	}
}

func (m *manager) evaluate(h uintptr) {
	if !isWindow(h) {
		delete(m.windows, h)
		return
	}
	if !m.cfg.RemoveTitleBars || !m.isCandidate(h) {
		return
	}
	w := m.windows[h]
	bits := zoneBits(h)
	if bits == 0 {
		if w != nil && !w.manual {
			m.showTitleBar(h, w, "left its zone")
		}
		return
	}
	if w == nil {
		w = m.track(h)
	}
	if !w.broken {
		m.handleSnapped(h, w, bits)
	}
}

func (m *manager) isCandidate(h uintptr) bool {
	if h == m.hwnd || !isWindowVisible(h) || isIconic(h) || rootWindow(h) != h {
		return false
	}
	if windowStyle(h)&wsChild != 0 || windowExStyle(h)&wsExToolWindow != 0 {
		return false
	}
	return !isCloaked(h) && windowPID(h) != m.ownPID
}

// track measures a window the first time it's seen, while it still has its own frame.
func (m *manager) track(h uintptr) *window {
	w := &window{
		exe:       processPath(windowPID(h)),
		titleBar:  drawsTitleBar(h),
		fixedSize: windowStyle(h)&wsThickFrame == 0,
	}
	if !w.titleBar {
		// An app's own bar is part of its content and can't be removed, only hidden above
		// the screen.
		w.ownBar = ownTitleBarHeight(h)
		w.offset = w.ownBar > 0
		if w.offset {
			log.Printf("%s draws its own %d px title bar; hiding it above the screen", m.describe(h, w), w.ownBar)
		}
	}
	m.windows[h] = w
	return w
}

func (m *manager) wantsBorderless(w *window) bool {
	return m.cfg.RemoveTitleBars && (w.titleBar || w.ownBar > 0) && !matchesApp(m.cfg.KeepTitleBarApps, w.exe)
}

func (m *manager) handleSnapped(h uintptr, w *window, bits uint64) {
	if !m.wantsBorderless(w) {
		m.showTitleBar(h, w, "kept")
		return
	}
	style := windowStyle(h)
	// A popup without its frame no longer qualifies for FancyZones, which would then stop
	// moving it on layout changes.
	if style&wsPopup != 0 || isZoomed(h) {
		return
	}
	zone, ok := m.zoneRect(h, bits)
	if !ok {
		return
	}
	framed := style&frameStyles != 0
	if framed && w.stripped {
		m.noteRefusal(h, w)
	}
	switch {
	case w.offset:
		m.placeClient(h, w, zone)
	case framed && w.fixedSize:
		m.strip(h, w, fixedSizeRect(h, zone))
	case framed:
		m.strip(h, w, zone)
	case !w.fixedSize:
		// Without a resize border FancyZones only moves the window; resize it to the zone.
		m.fit(h, w, zone)
	}
}

// fixedSizeRect decides how big a fixed-size window gets when its frame comes off. It keeps
// its client size at the zone's corner, except when that is close to the zone size: the
// title bar and Windows' maximum window height shave up to a few dozen pixels off a game
// that runs at exactly the zone size, so it gets the full zone back.
func fixedSizeRect(h uintptr, zone rect) rect {
	const tolerance = 64
	cw, ch := clientSize(h)
	if cw <= zone.width() && ch <= zone.height() && zone.width()-cw <= tolerance && zone.height()-ch <= tolerance {
		return zone
	}
	return rect{zone.Left, zone.Top, zone.Left + cw, zone.Top + ch}
}

// noteRefusal is called when a window we stripped has its frame again. Once is normal: games
// redo their window when the resolution changes. A frame that keeps coming back right after
// we removed it means the program enforces it, so stop fighting and use placeClient instead.
func (m *manager) noteRefusal(h uintptr, w *window) {
	if time.Since(w.strippedAt) > refusalWindow {
		w.refusals = 0
		return
	}
	w.refusals++
	if w.refusals < 2 {
		return
	}
	w.offset, w.stripped = true, false
	if w.squared {
		setCornerPreference(h, dwmwcpDefault)
		w.squared = false
	}
	log.Printf("%s keeps putting its title bar back; hiding it above the screen instead", m.describe(h, w))
}

// placeClient keeps the title bar and moves the window so the content below it covers the
// zone, with the frame sticking out around it. In a zone along the top of the monitor the
// title bar ends up above the screen, out of sight.
func (m *manager) placeClient(h uintptr, w *window, zone rect) {
	mi, ok := monitorInfo(monitorFromWindow(h))
	if !ok || zone.Top != mi.Monitor.Top {
		m.reportOnce(fmt.Sprintf("offset:%x", h), "%s: its title bar can only be hidden in zones along the top of the screen", m.describe(h, w))
		return
	}
	win, content := windowRect(h), contentRect(h, w)
	target := zone
	if w.fixedSize {
		target = rect{zone.Left, zone.Top, zone.Left + content.width(), zone.Top + content.height()}
	}
	m.fit(h, w, rect{
		Left:   target.Left - (content.Left - win.Left),
		Top:    target.Top - (content.Top - win.Top),
		Right:  target.Right + (win.Right - content.Right),
		Bottom: target.Bottom + (win.Bottom - content.Bottom),
	})
}

// contentRect is the part of the window below its title bar, in screen coordinates.
func contentRect(h uintptr, w *window) rect {
	if w.ownBar > 0 {
		r := visibleRect(h)
		r.Top += w.ownBar
		return r
	}
	return clientScreenRect(h)
}

// titleBarAboveScreen reports whether a window's top edge is above its monitor's work area.
func titleBarAboveScreen(h uintptr) bool {
	mi, ok := monitorInfo(monitorFromWindow(h))
	return ok && windowRect(h).Top < mi.Work.Top
}

// bringTitleBarBack moves a window down if its title bar is above the screen.
func bringTitleBarBack(h uintptr) {
	mi, ok := monitorInfo(monitorFromWindow(h))
	r := windowRect(h)
	if !ok || r.Top >= mi.Work.Top {
		return
	}
	d := mi.Work.Top - r.Top
	setWindowPos(h, rect{r.Left, r.Top + d, r.Right, r.Bottom + d}, swpQuiet)
}

func (m *manager) zoneRect(h uintptr, bits uint64) (rect, bool) {
	layout := m.layoutFor(monitorFromWindow(h))
	if layout.err != nil {
		m.reportOnce("layout:"+layout.err.Error(), "%v", layout.err)
		return rect{}, false
	}
	var out rect
	found := false
	for i := 0; i < 64; i++ {
		if bits&(1<<uint(i)) == 0 {
			continue
		}
		z, ok := layout.zones[i]
		if !ok {
			continue
		}
		if found {
			out = unionRect(out, z)
		} else {
			out, found = z, true
		}
	}
	if !found {
		m.reportOnce(fmt.Sprintf("zones:%s:%x", layout.name, bits), "layout %q has no zone %s", layout.name, zoneList(bits))
	}
	return out, found
}

func (m *manager) layoutFor(hmon uintptr) zoneLayout {
	if l, ok := m.layouts[hmon]; ok {
		return l
	}
	var l zoneLayout
	if mon, ok := identifyMonitor(hmon); ok {
		l = m.fz.layoutFor(mon, m.desktop)
	} else {
		l.err = fmt.Errorf("monitor %#x not found", hmon)
	}
	m.layouts[hmon] = l
	return l
}

// allow stops a tug of war with programs that undo every change: past the budget the window
// is left alone until the next layout or settings change.
func (m *manager) allow(h uintptr, w *window) bool {
	if w.gaveUp {
		return false
	}
	now := time.Now()
	recent := w.changes[:0]
	for _, t := range w.changes {
		if now.Sub(t) < changeBudgetWindow {
			recent = append(recent, t)
		}
	}
	w.changes = recent
	if len(w.changes) >= changeBudget {
		w.gaveUp = true
		log.Printf("%s keeps undoing changes; leaving it alone until the next layout change", m.describe(h, w))
		return false
	}
	w.changes = append(w.changes, now)
	return true
}

// strip removes the title bar and border and moves the window to target in the same step,
// so the program is resized only once.
func (m *manager) strip(h uintptr, w *window, target rect) bool {
	if !m.allow(h, w) {
		return false
	}
	style, ex := windowStyle(h), windowExStyle(h)
	if !w.stripped {
		w.origStyle, w.origExStyle = style, ex
	}
	if err := setStyles(h, style&^frameStyles, ex&^frameExStyles); err != nil {
		m.markBroken(h, w, "removing the title bar", err)
		return false
	}
	if m.cfg.SquareCorners {
		setCornerPreference(h, dwmwcpDoNotRound)
		w.squared = true
	}
	w.stripped, w.strippedAt = true, time.Now()
	if err := setWindowPos(h, target, swpQuiet|swpFrameChanged); err != nil {
		m.markBroken(h, w, "resizing", err)
		return false
	}
	log.Printf("%s: title bar removed, now %v", m.describe(h, w), target)
	return true
}

func (m *manager) fit(h uintptr, w *window, target rect) {
	if windowRect(h) == target || !m.allow(h, w) {
		return
	}
	if err := setWindowPos(h, target, swpQuiet); err != nil {
		m.markBroken(h, w, "resizing", err)
	}
}

// showTitleBar undoes whatever we did to the window's title bar.
func (m *manager) showTitleBar(h uintptr, w *window, why string) {
	if w.stripped {
		m.restore(h, w, why)
	}
	if !w.offset || !titleBarAboveScreen(h) {
		return
	}
	// A resizable window that is still snapped goes back to exactly its zone; anything else
	// is just moved down so its title bar is on screen again.
	if bits := zoneBits(h); bits != 0 && !w.fixedSize {
		if zone, ok := m.zoneRect(h, bits); ok {
			if err := setVisibleRect(h, zone); err != nil {
				log.Printf("%s: %v", m.describe(h, w), err)
			}
			return
		}
	}
	bringTitleBarBack(h)
}

func (m *manager) restore(h uintptr, w *window, why string) {
	visible := windowRect(h)
	style := windowStyle(h) | w.origStyle&frameStyles
	ex := windowExStyle(h) | w.origExStyle&frameExStyles
	if err := setStyles(h, style, ex); err != nil {
		m.markBroken(h, w, "restoring the title bar", err)
		return
	}
	if w.squared {
		setCornerPreference(h, dwmwcpDefault)
		w.squared = false
	}
	w.stripped, w.manual = false, false
	if err := setVisibleRect(h, visible); err != nil {
		log.Printf("%s: moving after restoring the title bar: %v", m.describe(h, w), err)
	}
	log.Printf("%s: title bar restored (%s)", m.describe(h, w), why)
}

func (m *manager) restoreAll(why string) {
	for h, w := range m.windows {
		if isWindow(h) {
			m.showTitleBar(h, w, why)
		}
	}
}

func (m *manager) markBroken(h uintptr, w *window, action string, err error) {
	w.broken = true
	log.Printf("%s: Windows refused %s (%v). If it runs as administrator, FancyBorderless has to as well.", m.describe(h, w), action, err)
}

func (m *manager) setEnabled(on bool) {
	m.cfg.RemoveTitleBars = on
	m.saveConfig()
	if on {
		m.refitAll()
		m.scan()
	} else {
		m.restoreAll("turned off")
	}
	m.updateTrayIcon()
}

func (m *manager) setSquareCorners(on bool) {
	m.cfg.SquareCorners = on
	m.saveConfig()
	m.applyCorners()
}

func (m *manager) applyCorners() {
	pref := uint32(dwmwcpDefault)
	if m.cfg.SquareCorners {
		pref = dwmwcpDoNotRound
	}
	for h, w := range m.windows {
		if w.stripped && isWindow(h) {
			setCornerPreference(h, pref)
			w.squared = m.cfg.SquareCorners
		}
	}
}

// setKeep remembers whether an app keeps its title bar and applies it to its open windows.
func (m *manager) setKeep(exeName string, keep bool) {
	list := []string{}
	for _, e := range m.cfg.KeepTitleBarApps {
		if !matchesApp([]string{e}, exeName) {
			list = append(list, e)
		}
	}
	if keep {
		list = append(list, exeName)
	}
	m.cfg.KeepTitleBarApps = list
	m.saveConfig()
	m.refitAll()
}

func (m *manager) registerHotkeys() {
	procUnregisterHotKey.Call(m.hwnd, hotkeyToggleTitleBar)
	spec := m.cfg.ToggleTitleBarHotkey
	if spec == "" {
		return
	}
	mods, vk, err := parseHotkey(spec)
	if err != nil {
		log.Printf("hotkey: %v", err)
		return
	}
	if r, _, err := procRegisterHotKey.Call(m.hwnd, hotkeyToggleTitleBar, uintptr(mods|modNoRepeat), uintptr(vk)); r == 0 {
		log.Printf("hotkey %s is already taken by another program (%v)", spec, err)
	}
}

// toggleTitleBar hides or shows the title bar of the window in front. The choice is
// remembered for the app, so its other windows and later ones follow it.
func (m *manager) toggleTitleBar() {
	h := rootWindow(foregroundWindow())
	if h == 0 || !m.isCandidate(h) {
		return
	}
	if !m.cfg.RemoveTitleBars {
		m.notify("FancyBorderless is turned off. Turn on \"Remove title bars\" in its tray menu.")
		return
	}
	w := m.windows[h]
	if w == nil {
		w = m.track(h)
	}
	if w.exe == "" {
		beep()
		return
	}
	exeName := filepath.Base(w.exe)
	name := displayName(exeName)
	w.broken, w.changes, w.gaveUp = false, nil, false

	if w.stripped || (w.offset && titleBarAboveScreen(h)) {
		m.setKeep(exeName, true)
		m.showTitleBar(h, w, "hotkey")
		m.notify(name + " keeps its title bar.")
		return
	}
	if !w.titleBar && w.ownBar == 0 {
		m.notify(name + " draws its own title bar with controls in it, so FancyBorderless leaves it alone.")
		return
	}
	m.setKeep(exeName, false)
	bits := zoneBits(h)
	switch {
	case bits != 0:
		m.handleSnapped(h, w, bits)
	case w.titleBar:
		if m.strip(h, w, visibleRect(h)) {
			w.manual = true
		}
	}
	switch {
	case w.stripped:
		m.notify(name + ": title bar removed.")
	case w.offset && titleBarAboveScreen(h):
		m.notify(name + ": title bar hidden above the screen.")
	case w.broken:
		m.notify("Windows refused to change " + name + ". If it runs as administrator, FancyBorderless has to as well.")
	case w.ownBar > 0 && bits == 0:
		m.notify("Snap " + name + " into a zone along the top of the screen to hide its title bar.")
	case w.offset:
		m.notify(name + "'s title bar can only be hidden in zones along the top of the screen.")
	default:
		m.notify("Couldn't remove the title bar of " + name + ". The log has details.")
	}
}

func (m *manager) shutdown() {
	for _, h := range m.hooks {
		procUnhookWinEvent.Call(h)
	}
	m.hooks = nil
	m.restoreAll("FancyBorderless exited")
	m.removeTrayIcon()
}

func (m *manager) reportOnce(key, format string, args ...any) {
	if m.reported[key] {
		return
	}
	m.reported[key] = true
	log.Printf(format, args...)
}

func (m *manager) describe(h uintptr, w *window) string {
	name := "?"
	if w.exe != "" {
		name = filepath.Base(w.exe)
	}
	return fmt.Sprintf("%s %q", name, windowTitle(h))
}

func zoneList(bits uint64) string {
	s := ""
	for i := 0; i < 64; i++ {
		if bits&(1<<uint(i)) != 0 {
			if s != "" {
				s += "+"
			}
			s += fmt.Sprint(i + 1)
		}
	}
	return s
}
