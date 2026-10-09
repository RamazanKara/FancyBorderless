package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	for _, tc := range []struct {
		name, data string
		wantError  bool
	}{
		{"defaults", `{}`, false},
		{"partial", `{"removeTitleBars":false}`, false},
		{"null lists", `{"keepTitleBarApps":null,"removeTitleBarApps":null}`, false},
		{"disabled hotkey", `{"toggleTitleBarHotkey":""}`, false},
		{"unknown field", `{"futureSetting":true}`, false},
		{"truncated", `{"removeTitleBars":`, true},
		{"wrong type", `{"keepTitleBarApps":"game"}`, true},
		{"invalid hotkey", `{"toggleTitleBarHotkey":"Ctrl+F25"}`, true},
		{"unmodified hotkey", `{"toggleTitleBarHotkey":"T"}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			oldPath := configPath
			configPath = filepath.Join(t.TempDir(), "config.json")
			t.Cleanup(func() { configPath = oldPath })
			if err := os.WriteFile(configPath, []byte(tc.data), 0600); err != nil {
				t.Fatal(err)
			}
			got, err := loadConfig()
			if (err != nil) != tc.wantError {
				t.Fatalf("loadConfig error = %v, want error %v", err, tc.wantError)
			}
			want := defaultConfig()
			if tc.name == "partial" {
				want.RemoveTitleBars = false
			}
			if tc.name == "disabled hotkey" {
				want.ToggleTitleBarHotkey = ""
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("config = %+v, want %+v", got, want)
			}
		})
	}
}

func TestConfigFirstRunAndRoundTrip(t *testing.T) {
	oldPath := configPath
	configPath = filepath.Join(t.TempDir(), "config.json")
	t.Cleanup(func() { configPath = oldPath })
	before := fileStamp(configPath)
	got, err := loadConfig()
	if err != nil || !reflect.DeepEqual(got, defaultConfig()) {
		t.Fatalf("first run = %+v, %v", got, err)
	}
	if fileStamp(configPath) == before {
		t.Fatal("creating the settings file did not change its stamp")
	}
	got.KeepTitleBarApps = []string{"Game", "Player.EXE"}
	got.RemoveTitleBarApps = []string{"Browser.exe"}
	got.ToggleTitleBarHotkey = "Win+PageUp"
	got.RunAsAdministrator = true
	if err := writeJSON(configPath, got); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadConfig()
	if err != nil || !reflect.DeepEqual(loaded, got) {
		t.Fatalf("round trip = %+v, %v; want %+v", loaded, err, got)
	}
	configPath = filepath.Join(configPath, "missing", "config.json")
	if _, err := loadConfig(); err == nil {
		t.Fatal("unwritable settings path was accepted")
	}
}

func TestParseHotkey(t *testing.T) {
	for _, tc := range []struct {
		spec     string
		mods, vk uint32
	}{
		{"Ctrl+Alt+Shift+T", modControl | modAlt | modShift, 'T'},
		{" control + win + z ", modControl | modWin, 'Z'},
		{"Alt+0", modAlt, '0'},
		{"Shift+9", modShift, '9'},
		{"Ctrl+F1", modControl, 0x70},
		{"Ctrl+F24", modControl, 0x87},
		{"Ctrl+Numpad0", modControl, 0x60},
		{"Ctrl+Numpad9", modControl, 0x69},
		{"Win+PageUp", modWin, 0x21},
		{"Ctrl+Space", modControl, 0x20},
	} {
		t.Run(tc.spec, func(t *testing.T) {
			mods, vk, err := parseHotkey(tc.spec)
			if err != nil || mods != tc.mods || vk != tc.vk {
				t.Fatalf("parseHotkey = %#x, %#x, %v; want %#x, %#x", mods, vk, err, tc.mods, tc.vk)
			}
		})
	}
	for _, spec := range []string{"", "T", "F1", "Ctrl", "Ctrl+", "+T", "Ctrl++T", "Meta+T", "Ctrl+F0", "Ctrl+F25", "Ctrl+Foo", "Alt+Numpad-1", "Alt+Numpad10", "Alt+NumpadX", "Ctrl+Escape", "Ctrl+é"} {
		t.Run("invalid "+spec, func(t *testing.T) {
			if _, _, err := parseHotkey(spec); err == nil {
				t.Fatalf("accepted %q", spec)
			}
		})
	}
}

func TestMatchesApp(t *testing.T) {
	for _, tc := range []struct {
		entry, path string
		want        bool
	}{
		{" game ", `C:\Games\GAME.exe`, true},
		{"Game.EXE", `C:\Games\game.exe`, true},
		{"game", `C:\Games\other.exe`, false},
		{"game", `C:\Games\game-helper.exe`, false},
		{"", `C:\Games\game.exe`, false},
	} {
		t.Run(tc.entry+"/"+tc.path, func(t *testing.T) {
			if got := matchesApp([]string{tc.entry}, tc.path); got != tc.want {
				t.Errorf("matchesApp = %v, want %v", got, tc.want)
			}
		})
	}
}
