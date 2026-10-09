package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSetKeepFromExtensionlessMenuEntry(t *testing.T) {
	for _, tc := range []struct {
		name       string
		busy, keep bool
	}{
		{"remove ordinary bar", false, false},
		{"keep ordinary bar", false, true},
		{"remove busy bar", true, false},
		{"keep busy bar", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			oldPath := configPath
			configPath = filepath.Join(t.TempDir(), "config.json")
			t.Cleanup(func() { configPath = oldPath })
			h, _ := ownedTestWindows(t)
			m := newManager()
			m.hwnd = h
			m.cfg = defaultConfig()
			m.cfg.KeepTitleBarApps = []string{" Player ", "other.exe"}
			m.cfg.RemoveTitleBarApps = []string{"PLAYER.exe"}
			m.windows[h] = &window{exe: `C:\Apps\Player.exe`, busyBar: tc.busy}
			apps := m.menuApps()
			if !reflect.DeepEqual(apps, []string{"other.exe", "Player"}) {
				t.Fatalf("menu apps = %v", apps)
			}
			m.setKeep(apps[1], tc.keep)
			procKillTimer.Call(m.hwnd, h)
			if got := m.keepsTitleBar(`C:\Apps\Player.exe`, tc.busy); got != tc.keep {
				t.Errorf("keepsTitleBar = %v, want %v", got, tc.keep)
			}
			wantKeep, wantRemove := []string{"other.exe"}, []string{}
			if tc.keep && !tc.busy {
				wantKeep = append(wantKeep, "Player.exe")
			}
			if !tc.keep && tc.busy {
				wantRemove = append(wantRemove, "Player.exe")
			}
			loaded, err := loadConfig()
			if err != nil || !reflect.DeepEqual(loaded.KeepTitleBarApps, wantKeep) || !reflect.DeepEqual(loaded.RemoveTitleBarApps, wantRemove) {
				t.Fatalf("saved config = %+v, %v; want lists %v / %v", loaded, err, wantKeep, wantRemove)
			}
		})
	}
}

func TestInvalidHotkeyReloadKeepsSettings(t *testing.T) {
	oldPath := configPath
	configPath = filepath.Join(t.TempDir(), "config.json")
	t.Cleanup(func() { configPath = oldPath })
	m := newManager()
	m.cfg = defaultConfig()
	m.cfg.KeepTitleBarApps = []string{"Game.exe"}
	m.cfgStamp = "previous settings"
	want := m.cfg
	if err := os.WriteFile(configPath, []byte(`{"removeTitleBars":false,"toggleTitleBarHotkey":"Ctrl+F25"}`), 0600); err != nil {
		t.Fatal(err)
	}
	m.reloadConfig()
	if !reflect.DeepEqual(m.cfg, want) {
		t.Fatalf("invalid reload replaced settings: %+v, want %+v", m.cfg, want)
	}
}
