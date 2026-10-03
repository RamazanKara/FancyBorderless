package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// The tray menu and the hotkey edit this file; editing it by hand works too and takes effect
// as soon as it's saved.
type config struct {
	RemoveTitleBars bool `json:"removeTitleBars"`
	// The user's choices per app. Apps in neither list lose their title bar unless it has
	// tabs or buttons in it, like a browser's.
	KeepTitleBarApps     []string `json:"keepTitleBarApps"`
	RemoveTitleBarApps   []string `json:"removeTitleBarApps"`
	ToggleTitleBarHotkey string   `json:"toggleTitleBarHotkey"`
	// RunAsAdministrator is needed for games that run as administrator: Windows doesn't let
	// a normal program move or restyle their windows.
	RunAsAdministrator bool `json:"runAsAdministrator"`
}

func defaultConfig() config {
	return config{
		RemoveTitleBars:    true,
		KeepTitleBarApps:   []string{},
		RemoveTitleBarApps: []string{},
		// Shift is included because Ctrl+Alt+<key> is AltGr+<key> on many keyboard layouts.
		ToggleTitleBarHotkey: "Ctrl+Alt+Shift+T",
	}
}

var (
	appDir     = filepath.Join(os.Getenv("APPDATA"), "FancyBorderless")
	configPath = filepath.Join(appDir, "config.json")
	logPath    = filepath.Join(appDir, "FancyBorderless.log")
)

// loadConfig reads the settings file and creates it with the defaults on first run.
func loadConfig() (config, error) {
	c := defaultConfig()
	err := readJSON(configPath, &c)
	if errors.Is(err, fs.ErrNotExist) {
		return c, writeJSON(configPath, c)
	}
	if err != nil {
		return defaultConfig(), err
	}
	if c.KeepTitleBarApps == nil {
		c.KeepTitleBarApps = []string{}
	}
	if c.RemoveTitleBarApps == nil {
		c.RemoveTitleBarApps = []string{}
	}
	return c, nil
}

func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// fileStamp changes whenever one of the files is rewritten.
func fileStamp(paths ...string) string {
	var b strings.Builder
	for _, p := range paths {
		if fi, err := os.Stat(p); err == nil {
			fmt.Fprintf(&b, "%d/%d;", fi.ModTime().UnixNano(), fi.Size())
		} else {
			b.WriteString("-;")
		}
	}
	return b.String()
}

// matchesApp compares exe file names case-insensitively; ".exe" is optional in the list.
func matchesApp(list []string, exePath string) bool {
	name := strings.ToLower(filepath.Base(exePath))
	for _, entry := range list {
		e := strings.ToLower(strings.TrimSpace(entry))
		if !strings.HasSuffix(e, ".exe") {
			e += ".exe"
		}
		if e == name {
			return true
		}
	}
	return false
}

// displayName turns "MyApp.exe" into "MyApp" for menus and notifications.
func displayName(exeName string) string {
	if strings.HasSuffix(strings.ToLower(exeName), ".exe") {
		return exeName[:len(exeName)-len(".exe")]
	}
	return exeName
}

var namedKeys = map[string]uint32{
	"left": 0x25, "up": 0x26, "right": 0x27, "down": 0x28,
	"pageup": 0x21, "pagedown": 0x22, "end": 0x23, "home": 0x24,
	"insert": 0x2D, "delete": 0x2E, "space": 0x20,
}

// parseHotkey turns "Ctrl+Alt+Shift+T" into RegisterHotKey modifiers and a virtual key.
func parseHotkey(spec string) (mods, vk uint32, err error) {
	parts := strings.Split(spec, "+")
	for i, part := range parts {
		p := strings.ToLower(strings.TrimSpace(part))
		if i < len(parts)-1 {
			switch p {
			case "ctrl", "control":
				mods |= modControl
			case "alt":
				mods |= modAlt
			case "shift":
				mods |= modShift
			case "win":
				mods |= modWin
			default:
				return 0, 0, fmt.Errorf("unknown modifier %q in %q", part, spec)
			}
			continue
		}
		switch {
		case len(p) == 1 && (p[0] >= 'a' && p[0] <= 'z' || p[0] >= '0' && p[0] <= '9'):
			vk = uint32(strings.ToUpper(p)[0])
		case strings.HasPrefix(p, "numpad"):
			n, convErr := strconv.Atoi(p[len("numpad"):])
			if convErr != nil || n < 0 || n > 9 {
				return 0, 0, fmt.Errorf("unknown key %q in %q", part, spec)
			}
			vk = 0x60 + uint32(n)
		case len(p) > 1 && p[0] == 'f':
			n, convErr := strconv.Atoi(p[1:])
			if convErr != nil || n < 1 || n > 24 {
				return 0, 0, fmt.Errorf("unknown key %q in %q", part, spec)
			}
			vk = 0x70 + uint32(n-1)
		default:
			k, ok := namedKeys[p]
			if !ok {
				return 0, 0, fmt.Errorf("unknown key %q in %q", part, spec)
			}
			vk = k
		}
	}
	if mods == 0 {
		// A hotkey without a modifier would take that key away from every other app.
		return 0, 0, fmt.Errorf("%q needs Ctrl, Alt, Shift or Win", spec)
	}
	return mods, vk, nil
}
