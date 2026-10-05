package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

const frameTestHostEnv = "FANCYBORDERLESS_FRAME_TEST"

// The child owns an unzoned test window. This exercises the same cross-process
// Windows calls as the utility, without managing any user windows or settings.
func TestFrameWindowHost(t *testing.T) {
	mode := os.Getenv(frameTestHostEnv)
	if mode == "" {
		return
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	procSetProcessDpiAwarenessContext.Call(dpiAwarenessPerMonitorV2)
	callback := syscall.NewCallback(func(h, msg, wp uintptr, lp unsafe.Pointer) uintptr {
		if msg == 0x83 && strings.HasPrefix(mode, "custom") { // WM_NCCALCSIZE
			// Own title bar, with the usual resize margins on the other three sides.
			r := (*rect)(lp)
			top := r.Top
			procDefWindowProcW.Call(h, msg, wp, uintptr(lp))
			r.Top = top
			return 0
		}
		if msg == wmDestroy {
			procPostQuitMessage.Call(0)
			return 0
		}
		r, _, _ := procDefWindowProcW.Call(h, msg, wp, uintptr(lp))
		return r
	})
	class := utf16Ptr("FancyBorderlessFrameTest")
	instance, _, _ := procGetModuleHandleW.Call(0)
	wc := wndClassEx{WndProc: callback, Instance: instance, ClassName: class}
	wc.Size = uint32(unsafe.Sizeof(wc))
	if r, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		t.Fatal(err)
	}
	h, _, err := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(class)), 0,
		wsCaption|wsThickFrame, 300, 300, 800, 600, 0, 0, instance, 0)
	if h == 0 {
		t.Fatal(err)
	}
	setFrameDrawing(h, !strings.HasSuffix(mode, "disabled"))
	fmt.Fprintln(os.Stdout, h)
	var msg winMsg
	getMessage := user32.NewProc("GetMessageW")
	for {
		r, _, _ := getMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			return
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func frameTestWindow(t *testing.T, mode string) uintptr {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^TestFrameWindowHost$")
	cmd.Env = append(os.Environ(), frameTestHostEnv+"="+mode)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	scanner := bufio.NewScanner(out)
	if !scanner.Scan() {
		t.Fatalf("test window did not start: %v", scanner.Err())
	}
	h, err := strconv.ParseUint(scanner.Text(), 10, 64)
	if err != nil {
		t.Fatalf("test window: %q: %v", scanner.Text(), err)
	}
	return uintptr(h)
}

func TestClippedFrameRestoresRendering(t *testing.T) {
	for _, mode := range []string{"standard", "custom", "custom_disabled"} {
		t.Run(mode, func(t *testing.T) {
			h := frameTestWindow(t, mode)
			wantDrawn := frameDrawn(h)
			beforeRegion, beforeHasRegion := regionBox(h)
			wantClient := clientScreenRect(h)
			m := newManager()
			w := &window{fixedSize: true}
			if strings.HasPrefix(mode, "custom") {
				w.ownBar = 32
			}
			for cycle := 0; cycle < 3; cycle++ {
				w.changes = nil
				m.placeClient(h, w, contentRect(h, w))
				if !w.hidden || !hasRegion(h) || frameDrawn(h) {
					t.Fatalf("cycle %d: window was not clipped with its frame disabled", cycle)
				}
				m.unhide(h, w)
				time.Sleep(50 * time.Millisecond)
				if got := frameDrawn(h); got != wantDrawn {
					t.Errorf("cycle %d: restored frame drawing=%v, want %v", cycle, got, wantDrawn)
				}
				region, has := regionBox(h)
				if w.hidden || has != beforeHasRegion || region != beforeRegion || getProp(h, propHidden) != 0 {
					t.Fatalf("cycle %d: clipping was not removed: region=%v/%v", cycle, region, has)
				}
				if got := clientScreenRect(h); got != wantClient {
					t.Errorf("cycle %d: client changed from %v to %v", cycle, wantClient, got)
				}
			}
		})
	}
}

func TestClippedFrameRecoveredAfterRestart(t *testing.T) {
	for _, tc := range []struct {
		name     string
		mode     string
		backdrop uint32
		legacy   bool
	}{
		{"drawn", "custom", 2, false},
		{"undrawn", "custom_disabled", 0, false},
		{"legacy", "custom", 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := frameTestWindow(t, tc.mode)
			wantDrawn := frameDrawn(h)
			setBackdrop(h, tc.backdrop)
			if got := backdrop(h); got != tc.backdrop {
				t.Fatalf("test backdrop not available: got %d, want %d", got, tc.backdrop)
			}
			m := newManager()
			w := &window{fixedSize: true, ownBar: 32}
			m.placeClient(h, w, contentRect(h, w))
			if !w.hidden {
				t.Fatal("window was not clipped")
			}
			if tc.legacy {
				setProp(h, propHidden, 1) // A window left behind by 1.4.1.
			}
			// A fresh manager must recover the original state from the window alone.
			m = newManager()
			w = m.track(h)
			m.showTitleBar(h, w, "test restart")
			if frameDrawn(h) != wantDrawn || backdrop(h) != tc.backdrop {
				t.Errorf("restored frame/backdrop=%v/%d, want %v/%d", frameDrawn(h), backdrop(h), wantDrawn, tc.backdrop)
			}
			if w.hidden || hasRegion(h) || getProp(h, propHidden) != 0 {
				t.Fatal("recovered window is still clipped")
			}
		})
	}
}
