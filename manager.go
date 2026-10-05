package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"syscall"
	"time"
)

const (
	// Timer ids. Windows are evaluated on a timer of their own, with the window handle as its
	// id; handles are never this small.
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
	pid  uint32
	exe  string
	bits uint64 // zone marker when last evaluated
	// zoneSource is an owned content window carrying the frame's FancyZones marker.
	zoneSource uintptr
	// titleBar: Windows draws a title bar above the window's content.
	titleBar bool
	// ownBar: height of a title bar the app draws itself, plain (WPF and UWP apps, for
	// example) or busy with tabs and buttons (browsers). Busy ones are kept unless the user
	// picks the app with the hotkey or the tray menu.
	ownBar  int32
	busyBar bool
	// fixedSize: the window had no resize border of its own. FancyZones never resizes such
	// windows and neither do we; games in windowed mode are usually like this, and most of
	// them keep drawing at their resolution, so a smaller window would cut the picture off.
	fixedSize   bool
	origStyle   uint32
	origExStyle uint32
	// origRect and origVisible: the window and its visible part before its frame came off,
	// so a game gets exactly that window back with its frame.
	origRect, origVisible rect
	stripped              bool // title bar and border removed by us
	strippedAt            time.Time
	refusals              int    // times the program put its frame back right away
	offset                bool   // title bar kept but hidden (enforced frame, popup, or the app's own bar)
	hidden                bool   // title bar clipped off by us: window region set, frame and backdrop off
	backdrop              uint32 // the backdrop it had before
	frameWasDrawn         bool   // whether Windows drew its frame before clipping
	manual                bool   // title bar removed with the hotkey on a window that isn't snapped
	broken                bool   // Windows refused a change, usually because the window runs elevated
	gaveUp                bool
	changes               []time.Time
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
	reported map[string]bool
	// framedApps won't do without their frame (see rememberFrame), by exe path.
	framedApps map[string]bool
	hooks      []uintptr
	// moveHooks listen for window moves of the processes we manage, one hook per process.
	// A system-wide move hook would also fire on every mouse movement.
	moveHooks map[uint32]uintptr
}

func newManager() *manager {
	return &manager{
		ownPID:     uint32(os.Getpid()),
		fz:         newFancyZones(),
		layouts:    map[uintptr]zoneLayout{},
		windows:    map[uintptr]*window{},
		reported:   map[string]bool{},
		framedApps: map[string]bool{},
		moveHooks:  map[uint32]uintptr{},
	}
}

// WINEVENT_OUTOFCONTEXT: Windows queues the events to this thread's message loop instead of
// loading code into the processes that raise them.
var winEventCallback = syscall.NewCallback(func(hook, event, hwnd, idObject, idChild, thread, eventTime uintptr) uintptr {
	defer recoverCallback()
	if hwnd != 0 && int32(idObject) == objidWindow && int32(idChild) == childidSelf {
		app.onEvent(uint32(event), hwnd)
	}
	return 0
})

func setWinEventHook(event uint32, pid uint32) uintptr {
	h, _, err := procSetWinEventHook.Call(uintptr(event), uintptr(event), 0, winEventCallback, uintptr(pid), 0, winEventOutOfContext|winEventSkipOwnProcess)
	if h == 0 {
		log.Printf("SetWinEventHook %#x: %v", event, err)
	}
	return h
}

// hook listens system-wide only for events that are rare: a window appearing and the end of
// a mouse drag.
func (m *manager) hook() {
	for _, event := range []uint32{eventObjectShow, eventSystemMoveSizeEnd} {
		if h := setWinEventHook(event, 0); h != 0 {
			m.hooks = append(m.hooks, h)
		}
	}
}

func (m *manager) watchMoves(pid uint32) {
	if _, ok := m.moveHooks[pid]; ok || pid == 0 {
		return
	}
	if h := setWinEventHook(eventObjectLocationChange, pid); h != 0 {
		m.moveHooks[pid] = h
	}
}

func (m *manager) onEvent(event uint32, h uintptr) {
	switch event {
	case eventObjectShow:
		// Tooltips, menus and the controls inside windows show all the time; only windows
		// FancyZones could snap matter.
		if rootWindow(h) != h || windowExStyle(h)&wsExToolWindow != 0 {
			return
		}
	case eventObjectLocationChange:
		// Moves of windows we manage, and of other windows of the same app once FancyZones
		// snaps them (it doesn't always write its files for those).
		if rootWindow(h) != h {
			return
		}
		if _, ok := m.windows[frameWindow(h)]; !ok && zoneBits(h) == 0 {
			return
		}
	}
	m.schedule(h, settleDelay)
}

// schedule evaluates a window after a delay. Scheduling it again before then restarts the
// wait, so a burst of moves leads to one evaluation.
func (m *manager) schedule(h uintptr, delay time.Duration) {
	procSetTimer.Call(m.hwnd, h, uintptr(delay.Milliseconds()), 0)
}

func (m *manager) onTimer(id uintptr) {
	switch id {
	case timerScan:
		m.scan()
	case timerRefit:
		procKillTimer.Call(m.hwnd, timerRefit)
		m.layouts = map[uintptr]zoneLayout{}
		m.refitAll()
	default:
		procKillTimer.Call(m.hwnd, id)
		m.evaluate(id)
	}
}

// scheduleRefit waits for a display or work area change to finish before refitting.
func (m *manager) scheduleRefit() {
	procSetTimer.Call(m.hwnd, timerRefit, 500, 0)
}

// onFancyZonesChanged runs when FancyZones writes to its folder: a layout switch or a snap.
// FancyZones records snaps in app-zone-history.json, so windows snapped with the keyboard
// are usually picked up right away.
func (m *manager) onFancyZonesChanged() {
	m.reloadIfChanged()
	m.findNewlySnapped()
}

// findNewlySnapped looks for snapped windows we don't know yet. Windows we already manage
// are kept up to date by their move hooks.
func (m *manager) findNewlySnapped() {
	for _, h := range topLevelWindows() {
		if !isWindowVisible(h) || zoneBits(h) == 0 {
			continue
		}
		target := frameWindow(h)
		if w := m.windows[target]; w == nil || target != h && w.zoneSource != h {
			m.evaluate(h)
		}
	}
}

// scan is the safety net for anything no event reports, such as a virtual desktop switch.
func (m *manager) scan() {
	m.reloadIfChanged()
	if d := currentDesktop(); d != m.desktop {
		m.desktop = d
		m.layouts = map[uintptr]zoneLayout{}
		m.refitAll()
	}
	m.findNewlySnapped()
	pids := map[uint32]bool{}
	for h, w := range m.windows {
		if !isWindow(h) {
			delete(m.windows, h)
			continue
		}
		pids[w.pid] = true
		// Some apps take the region off when they redraw their frame; it's put back here.
		if m.zoneBits(h) != w.bits || w.hidden && !hasRegion(h) {
			m.schedule(h, 0)
		}
	}
	for pid, hook := range m.moveHooks {
		if !pids[pid] {
			procUnhookWinEvent.Call(hook)
			delete(m.moveHooks, pid)
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
}

func (m *manager) reloadConfig() {
	first := m.cfgStamp == ""
	m.cfgStamp = fileStamp(configPath)
	cfg, err := loadConfig()
	if err != nil {
		log.Printf("settings: %v", err)
		if !first {
			// A typo shouldn't throw away the app lists: carry on with what was loaded.
			m.notify("The settings file has an error, so FancyBorderless keeps its current settings. The log has details.")
			return
		}
		beep()
	}
	m.applyConfig(cfg)
	log.Print("settings loaded")
}

// applyConfig switches to new settings and brings the windows in line with them.
func (m *manager) applyConfig(cfg config) {
	old := m.cfg
	m.cfg = cfg
	m.registerHotkeys()
	if old.RemoveTitleBars && !cfg.RemoveTitleBars {
		m.restoreAll("turned off")
	}
	m.refitAll()
	if !old.RemoveTitleBars && cfg.RemoveTitleBars && m.fzStamp != "" {
		// Windows snapped while it was off haven't been looked at yet.
		m.findNewlySnapped()
	}
	m.updateTrayIcon()
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
	for h, w := range m.windows {
		w.changes, w.gaveUp = nil, false
		m.schedule(h, 0)
	}
}

func (m *manager) evaluate(h uintptr) {
	source := h
	h = frameWindow(h)
	if !isWindow(h) {
		delete(m.windows, h)
		return
	}
	if !m.cfg.RemoveTitleBars || !m.isCandidate(h) {
		return
	}
	w := m.windows[h]
	if w == nil && source != h {
		w = m.track(h)
	}
	if w != nil && source != h {
		w.zoneSource = source
	}
	bits := m.zoneBits(h)
	if w != nil {
		w.bits = bits
	}
	if bits == 0 {
		if w != nil && !w.manual {
			m.showTitleBar(h, w, "left its zone")
		}
		return
	}
	if w == nil {
		w = m.track(h)
		w.bits = bits
	}
	if !w.broken {
		m.handleSnapped(h, w, bits)
	}
}

// zoneBits follows the content window's marker without copying it to the frame.
// A video surface can retain an old marker after its frame is dragged elsewhere.
// Only inherit it while the frame actually fits that zone; never move a frame to
// an old location just because its video still remembers it.
func (m *manager) zoneBits(h uintptr) uint64 {
	if bits := zoneBits(h); bits != 0 {
		return bits
	}
	if w := m.windows[h]; w != nil && w.zoneSource != 0 && frameWindow(w.zoneSource) == h {
		bits := zoneBits(w.zoneSource)
		if bits == 0 {
			return 0
		}
		zone, ok := m.zoneRect(h, bits)
		bounds := visibleRect(h)
		if w.hidden {
			bounds = contentRect(h, w)
		}
		if ok && nearRect(bounds, zone, scaleForWindow(h, 8)) {
			return bits
		}
	}
	return 0
}

func (m *manager) isCandidate(h uintptr) bool {
	if h == m.hwnd || !isWindowVisible(h) || isIconic(h) || rootWindow(h) != h {
		return false
	}
	if windowStyle(h)&wsChild != 0 || windowExStyle(h)&wsExToolWindow != 0 {
		return false
	}
	return !isCloaked(h) && windowPID(h) != m.ownPID && !isHung(h)
}

// track measures a window the first time it's seen, while it still has its own frame, or
// picks up where an earlier FancyBorderless instance left off.
func (m *manager) track(h uintptr) *window {
	pid := windowPID(h)
	w := &window{pid: pid, exe: processPath(pid)}
	m.windows[h] = w
	if saved := getProp(h, propSavedStyle); saved != 0 {
		w.stripped, w.origStyle, w.origExStyle = true, uint32(saved>>32), uint32(saved)
		w.titleBar, w.fixedSize = true, w.origStyle&wsThickFrame == 0
		log.Printf("%s: title bar was removed by an earlier FancyBorderless", m.describe(h, w))
		return w
	}
	w.titleBar = drawsTitleBar(h)
	w.fixedSize = windowStyle(h)&wsThickFrame == 0
	if saved := getProp(h, propHidden); saved != 0 {
		w.hidden = true // by an earlier FancyBorderless
		w.frameWasDrawn = saved&2 == 0
		w.backdrop = uint32(saved >> 2)
	}
	w.offset = w.hidden || getProp(h, propKeepsFrame) != 0 || m.framedApps[w.exe]
	if !w.titleBar {
		m.measureOwnBar(h, w)
	} else if windowStyle(h)&wsPopup != 0 {
		// A popup without its frame no longer qualifies for FancyZones, which would then stop
		// moving it, so its title bar is hidden instead of removed.
		w.offset = true
		log.Printf("%s is a popup window; hiding its title bar", m.describe(h, w))
	}
	return w
}

// measureOwnBar looks for a title bar the app draws itself. It's part of the app's content,
// so it can't be removed, only hidden.
func (m *manager) measureOwnBar(h uintptr, w *window) {
	w.ownBar, w.busyBar = ownTitleBar(h)
	w.offset = w.ownBar > 0
	switch {
	case w.busyBar:
		log.Printf("%s draws its own %d px title bar with tabs or buttons in it", m.describe(h, w), w.ownBar)
	case w.offset:
		log.Printf("%s draws its own %d px title bar; hiding it", m.describe(h, w), w.ownBar)
	}
}

// keepsTitleBar is the user's choice for an app, or else the default: a title bar with tabs
// or buttons in it stays, any other goes.
func (m *manager) keepsTitleBar(exe string, busyBar bool) bool {
	switch {
	case matchesApp(m.cfg.KeepTitleBarApps, exe):
		return true
	case matchesApp(m.cfg.RemoveTitleBarApps, exe):
		return false
	}
	return busyBar
}

func (m *manager) wantsBorderless(w *window) bool {
	return m.cfg.RemoveTitleBars && (w.titleBar || w.ownBar > 0) && !m.keepsTitleBar(w.exe, w.busyBar)
}

func (m *manager) handleSnapped(h uintptr, w *window, bits uint64) {
	if !m.wantsBorderless(w) {
		m.showTitleBar(h, w, "kept")
		return
	}
	// From here on the window is managed, so react when it moves or changes its frame.
	m.watchMoves(w.pid)
	style := windowStyle(h)
	if isZoomed(h) {
		// A region would cut a maximized window down to its old size.
		if w.hidden {
			m.unhide(h, w)
		}
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
	removeProp(h, propSavedStyle)
	m.rememberFrame(h, w)
	log.Printf("%s keeps putting its title bar back; hiding it instead", m.describe(h, w))
}

// rememberFrame notes that a program won't do without its frame, on the window (which keeps
// the note through a restart of FancyBorderless) and for the app until FancyBorderless exits,
// so it isn't stripped again: some games take every attempt as a new window size.
func (m *manager) rememberFrame(h uintptr, w *window) {
	setProp(h, propKeepsFrame, 1)
	m.framedApps[w.exe] = true
}

// placeClient keeps the title bar but moves the window so the content below it covers the
// zone, and clips everything around the content off with a window region, in any zone.
// Windows draws its frame and backdrop on top of any region, so those are switched off while
// the window is clipped. The region goes on before the window moves, so the title bar never
// shows outside the zone on its way, on a monitor above for instance.
func (m *manager) placeClient(h uintptr, w *window, zone rect) {
	if !w.hidden {
		if hasRegion(h) {
			// The app shapes its window itself (a skinned player, say); leave that to it.
			m.reportOnce(fmt.Sprintf("region:%x", h), "%s shapes its own window, so its title bar stays", m.describe(h, w))
			return
		}
		w.backdrop = backdrop(h)
		w.frameWasDrawn = frameDrawn(h)
	}
	clip := func() bool {
		if err := clipTo(h, contentRect(h, w)); err != nil {
			m.unhide(h, w)
			m.markBroken(h, w, "hiding the title bar", err)
			return false
		}
		return true
	}
	if !clip() {
		return
	}
	if frameDrawn(h) {
		setFrameDrawing(h, false)
	}
	if backdrop(h) != dwmsbtNone {
		setBackdrop(h, dwmsbtNone)
	}
	// Apps that redraw their frame when its drawing changes may drop the region meanwhile.
	if !clip() {
		return
	}
	win, content := windowRect(h), contentRect(h, w)
	target := zone
	if w.fixedSize {
		target = rect{zone.Left, zone.Top, zone.Left + content.width(), zone.Top + content.height()}
	}
	target = rect{
		Left:   target.Left - (content.Left - win.Left),
		Top:    target.Top - (content.Top - win.Top),
		Right:  target.Right + (win.Right - content.Right),
		Bottom: target.Bottom + (win.Bottom - content.Bottom),
	}
	if target.width() != win.width() || target.height() != win.height() {
		// Resize in place first: some apps (Chromium-based ones) drop the region when their
		// size changes, and that mustn't happen with the title bar already above the zone.
		m.fit(h, w, rect{win.Left, win.Top, win.Left + target.width(), win.Top + target.height()})
		if !clip() {
			return
		}
	}
	m.fit(h, w, target)
	if !clip() {
		return
	}
	if !w.hidden {
		w.hidden = true
		saved := uintptr(w.backdrop)<<2 | 1
		if !w.frameWasDrawn {
			saved |= 2
		}
		setProp(h, propHidden, saved)
	}
}

// unhide undoes the clipping without moving the window.
func (m *manager) unhide(h uintptr, w *window) {
	unclipWindow(h)
	setBackdrop(h, w.backdrop)
	setFrameDrawing(h, w.frameWasDrawn)
	w.hidden = false
	removeProp(h, propHidden)
}

// contentRect is the part of the window below its title bar, in screen coordinates. It goes
// by the client area, which stays the same when Windows' frame drawing is switched off.
func contentRect(h uintptr, w *window) rect {
	r := clientScreenRect(h)
	if w.ownBar > 0 {
		r.Top = visibleRect(h).Top + w.ownBar
	}
	return r
}

// bringTitleBarBack moves a window down if its title bar is above the screen.
func bringTitleBarBack(h uintptr) {
	mi, ok := monitorInfo(monitorFromWindow(h))
	r := windowRect(h)
	if !ok || isZoomed(h) || r.Top >= mi.Work.Top {
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
func (m *manager) strip(h uintptr, w *window, target rect) {
	if !m.allow(h, w) {
		return
	}
	style, ex := windowStyle(h), windowExStyle(h)
	if !w.stripped {
		w.origStyle, w.origExStyle = style, ex
		w.origRect, w.origVisible = windowRect(h), visibleRect(h)
	}
	if err := setStyles(h, style&^frameStyles, ex&^frameExStyles); err != nil {
		m.markBroken(h, w, "removing the title bar", err)
		return
	}
	w.stripped, w.strippedAt = true, time.Now()
	setProp(h, propSavedStyle, uintptr(w.origStyle)<<32|uintptr(w.origExStyle))
	if err := setWindowPos(h, target, swpQuiet|swpFrameChanged); err != nil {
		m.markBroken(h, w, "resizing", err)
		return
	}
	// Some games size their window right back to what it is with a frame (in the call above,
	// before it returns) and would draw their picture into a window bigger than it. They get
	// the frame back straight away and their title bar is clipped off instead.
	if r := windowRect(h); w.fixedSize && (r.width()-target.width() > 2 || r.height()-target.height() > 2) {
		m.restore(h, w, "it sizes its window for its frame")
		w.offset = true
		m.rememberFrame(h, w)
		return
	}
	// Apps that draw their own frame (most browsers do) work out their borders themselves and
	// keep them without a frame style, which would leave a gap at the sides. A new window can
	// look like a normal one for a moment while it's being set up, so this is checked on the
	// result. A gap on one side only is a scroll bar.
	if win, client := windowRect(h), clientScreenRect(h); client.Left > win.Left && client.Right < win.Right {
		m.restore(h, w, "it draws its own frame")
		w.titleBar = false
		m.measureOwnBar(h, w)
		return
	}
	log.Printf("%s: title bar removed, now %v", m.describe(h, w), target)
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
	if !w.hidden {
		return
	}
	// The window moves back first, while it's still clipped, so the title bar doesn't show
	// outside its zone on the way. A snapped window moves down by its title bar; anything
	// else only if its title bar is above the screen.
	bits := m.zoneBits(h)
	if bits != 0 {
		r := windowRect(h)
		bar := contentRect(h, w).Top - r.Top
		setWindowPos(h, rect{r.Left, r.Top + bar, r.Right, r.Bottom + bar}, swpQuiet)
	} else {
		bringTitleBarBack(h)
	}
	m.unhide(h, w)
	log.Printf("%s: title bar shown again (%s)", m.describe(h, w), why)
	// A resizable window then fits its zone exactly again, frame included.
	if bits != 0 && !w.fixedSize {
		if zone, ok := m.zoneRect(h, bits); ok {
			if err := setVisibleRect(h, zone); err != nil {
				log.Printf("%s: %v", m.describe(h, w), err)
			}
		}
	}
}

func (m *manager) restore(h uintptr, w *window, why string) {
	now := windowRect(h)
	style := windowStyle(h) | w.origStyle&frameStyles
	ex := windowExStyle(h) | w.origExStyle&frameExStyles
	if err := setStyles(h, style, ex); err != nil {
		m.markBroken(h, w, "restoring the title bar", err)
		return
	}
	w.stripped, w.manual = false, false
	removeProp(h, propSavedStyle)
	var err error
	if o, v := w.origRect, w.origVisible; w.fixedSize && o != (rect{}) {
		// A game draws at a size of its own, so it gets exactly the window it had before, with
		// its visible corner where the window is now. That's one move: some games take any
		// size they see along the way as their new picture size.
		left, top := now.Left-(v.Left-o.Left), now.Top-(v.Top-o.Top)
		err = setWindowPos(h, rect{left, top, left + o.width(), top + o.height()}, swpQuiet|swpFrameChanged)
	} else {
		err = setVisibleRect(h, now)
	}
	if err != nil {
		log.Printf("%s: moving after restoring the title bar: %v", m.describe(h, w), err)
	}
	log.Printf("%s: title bar restored (%s)", m.describe(h, w), why)
}

func (m *manager) restoreAll(why string) {
	for h, w := range m.windows {
		if isWindow(h) && !isHung(h) {
			m.showTitleBar(h, w, why)
		}
	}
}

func (m *manager) markBroken(h uintptr, w *window, action string, err error) {
	w.broken = true
	log.Printf("%s: Windows refused %s (%v)", m.describe(h, w), action, err)
	if !isElevated() && processElevated(windowPID(h)) {
		m.notifyElevated(w.exe)
	}
}

// notifyElevated explains, once per app, why a window that runs as administrator can't be
// changed and what to turn on.
func (m *manager) notifyElevated(exe string) {
	key := "elevated:" + exe
	if m.reported[key] {
		return
	}
	m.reported[key] = true
	m.notify(elevatedHint(exe))
}

func elevatedHint(exe string) string {
	return displayName(filepath.Base(exe)) + " runs as administrator. Turn on \"Run as administrator\" in this tray menu, and \"Always run as administrator\" in PowerToys (General) so FancyZones can snap it."
}

func (m *manager) setRunAsAdministrator(on bool) {
	m.cfg.RunAsAdministrator = on
	m.saveConfig()
	if on && !isElevated() {
		if err := relaunchElevated(true); err != nil {
			m.cfg.RunAsAdministrator = false
			m.saveConfig()
			m.notify(err.Error())
			return
		}
		// The new instance waits for this one to exit.
		procPostMessageW.Call(m.hwnd, wmClose, 0, 0)
		return
	}
	m.syncStartup()
	if !on && isElevated() {
		m.notify("FancyBorderless keeps administrator rights until it's restarted.")
	}
}

// syncStartup moves "Start with Windows" between the Run key and the logon task when the
// administrator setting changed. Creating or removing the task needs administrator rights.
func (m *manager) syncStartup() {
	if !startsWithWindows() || !isElevated() {
		return
	}
	admin := m.cfg.RunAsAdministrator
	if logonTaskExists() == admin && runKeyEnabled() != admin {
		return
	}
	if err := setStartWithWindows(true, admin); err != nil {
		log.Printf("Start with Windows: %v", err)
	}
}

func (m *manager) toggleStartWithWindows() {
	admin := m.cfg.RunAsAdministrator && isElevated()
	if err := setStartWithWindows(!startsWithWindows(), admin); err != nil {
		m.notify("Couldn't change Start with Windows: " + err.Error())
	}
}

func (m *manager) setEnabled(on bool) {
	cfg := m.cfg
	cfg.RemoveTitleBars = on
	m.applyConfig(cfg)
	m.saveConfig()
}

// setKeep remembers whether an app keeps its title bar and applies it to its open windows.
// Only a choice that differs from the app's default is written down, so the lists stay short.
func (m *manager) setKeep(exeName string, keep bool) {
	isApp := func(e string) bool { return matchesApp([]string{e}, exeName) }
	m.cfg.KeepTitleBarApps = slices.DeleteFunc(m.cfg.KeepTitleBarApps, isApp)
	m.cfg.RemoveTitleBarApps = slices.DeleteFunc(m.cfg.RemoveTitleBarApps, isApp)
	switch busy := m.hasBusyBar(exeName); {
	case keep && !busy:
		m.cfg.KeepTitleBarApps = append(m.cfg.KeepTitleBarApps, exeName)
	case !keep && busy:
		m.cfg.RemoveTitleBarApps = append(m.cfg.RemoveTitleBarApps, exeName)
	}
	m.saveConfig()
	m.refitAll()
}

// hasBusyBar reports whether an open window of the app has tabs or buttons in its title bar,
// which it then keeps by default.
func (m *manager) hasBusyBar(exeName string) bool {
	for h, w := range m.windows {
		if w.busyBar && matchesApp([]string{exeName}, w.exe) && isWindow(h) {
			return true
		}
	}
	return false
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
	h := frameWindow(foregroundWindow())
	if h == 0 || !m.isCandidate(h) {
		return
	}
	if !m.cfg.RemoveTitleBars {
		m.notify("FancyBorderless is turned off. Turn on \"Remove title bars\" in its tray menu.")
		return
	}
	if !isElevated() && processElevated(windowPID(h)) {
		m.notify(elevatedHint(processPath(windowPID(h))))
		return
	}
	w := m.windows[h]
	if w == nil {
		w = m.track(h)
	}
	// The hotkey can arrive before the first scan, with the frame itself focused.
	if m.zoneBits(h) == 0 {
		for _, source := range topLevelWindows() {
			if source != h && zoneBits(source) != 0 && frameWindow(source) == h {
				w.zoneSource = source
				break
			}
		}
	}
	if w.exe == "" {
		beep()
		return
	}
	exeName := filepath.Base(w.exe)
	name := displayName(exeName)
	w.broken, w.changes, w.gaveUp = false, nil, false

	if w.stripped || w.hidden {
		m.setKeep(exeName, true)
		m.showTitleBar(h, w, "hotkey")
		m.notify(name + " keeps its title bar.")
		return
	}
	if !w.titleBar && w.ownBar == 0 {
		m.notify(name + " has no title bar to remove.")
		return
	}
	m.setKeep(exeName, false)
	bits := m.zoneBits(h)
	switch {
	case bits != 0:
		m.handleSnapped(h, w, bits)
	case w.titleBar && !w.offset:
		// Outside a zone only a frame that comes off can go. A window that keeps its frame
		// isn't tried again: some games take every attempt as a new window size.
		m.strip(h, w, visibleRect(h))
		w.manual = w.stripped
	}
	switch {
	case w.stripped:
		m.notify(name + ": title bar removed.")
	case w.hidden:
		m.notify(name + ": title bar hidden.")
	case w.broken:
		m.notify("Windows refused to change " + name + ". The log has details.")
	case w.offset && bits == 0:
		m.notify("Snap " + name + " into a zone to hide its title bar.")
	default:
		m.notify("Couldn't remove the title bar of " + name + ". The log has details.")
	}
}

func (m *manager) shutdown() {
	for _, h := range m.hooks {
		procUnhookWinEvent.Call(h)
	}
	for _, h := range m.moveHooks {
		procUnhookWinEvent.Call(h)
	}
	m.hooks, m.moveHooks = nil, map[uint32]uintptr{}
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
