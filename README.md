<img src="docs/icon.png" alt="" width="72" align="right">

# FancyBorderless

**Borderless windows for FancyZones.** Remove or hide the title bar of supported snapped windows so their content can fill the zone.

![A game and a media player snapped with FancyZones, first with title bars and the bottom of the game cut off, then borderless with everything in view](docs/demo.gif)

FancyBorderless runs in the Windows system tray and follows windows snapped by PowerToys FancyZones. It removes ordinary window frames and can clip detected custom title bars. Compatibility depends on how each app draws and sizes its windows.

## Highlights

- **Layout support** includes the built-in templates and custom grid and canvas layouts.
- **Fixed-size games keep their size** when switching layouts; they may extend beyond a smaller zone. Choose a windowed resolution that fits the zone.
- **Resizable windows follow layout changes** when FancyBorderless manages them.
- **Detected custom title bars can be clipped** while the window is snapped. Windows with an existing custom region are left alone by the clipping path.
- **Title bars with detected tabs or buttons stay by default.** The hotkey or tray menu can override this choice for an app.
- **One hotkey** (Ctrl+Alt+Shift+T) removes or restores supported title bars and remembers your choice for that app. Custom title bars need a snapped window.
- **Settings are in the tray menu**, including Start with Windows.
- **Administrator mode** allows access to elevated windows; PowerToys also needs administrator rights to snap them.
- **Event-driven updates** are backed by a five-second scan.
- **No code injection.** Window changes use Windows APIs, and normal exit attempts to restore managed windows.

## Getting started

1. Install [PowerToys](https://github.com/microsoft/PowerToys) and turn on FancyZones.
2. Download `FancyBorderless.exe` from the [latest release](https://github.com/RamazanKara/FancyBorderless/releases/latest) and run it. It lives in the system tray.
3. Snap windows the way you always do: Shift-drag, or Win+arrow keys if **Override Windows Snap** is on in FancyZones' settings. FancyBorderless takes it from there.
4. Click the tray icon and tick **Start with Windows** to have it ready every time you sign in.

You can also build the executable from source with the command below.

## Tips for games

- Set the game to **Windowed** mode at a resolution that fits its zone. Allow for zone spacing and the taskbar; a zone may be smaller than half the monitor. FancyBorderless leaves detected fullscreen windows alone.
- Snap it into its zone once. FancyZones remembers the zone and puts the game back there on every launch ("Move newly created windows to their last known zone" in FancyZones' settings).
- Keep games off FancyZones' excluded apps list, so FancyZones can snap them.

### Games that run as administrator

Windows restricts changes to windows owned by elevated processes, so FancyZones and FancyBorderless both need administrator mode to work with those windows:

1. In PowerToys Settings, General tab, turn on **Always run as administrator** ([PowerToys docs](https://learn.microsoft.com/windows/powertoys/administrator)).
2. In FancyBorderless's tray menu, tick **Run as administrator** and confirm the Windows prompt.

In administrator mode, Start with Windows uses a scheduled task. The hotkey reports when it detects that a window requires administrator rights.

## Tray menu

- Remove title bars (turns everything on or off)
- Start with Windows
- Run as administrator
- Keep title bar for (configured apps and tracked windows with detected title bars; ticked apps keep their title bar)
- Open settings file, Open log, Exit

Hotkey actions report their result through tray notifications and the log. Windows notification settings can affect whether a notification is shown.

## How it works

FancyBorderless works entirely from the outside, using standard Windows APIs.

- FancyZones marks every window it snaps with a window property (`FancyZones_zones`) holding the zones it's in. FancyBorderless reads that property to know which windows are snapped and where.
- It reads FancyZones' layout files (`applied-layouts.json`, `custom-layouts.json`) and calculates zone rectangles using a port of [PowerToys' layout calculations](https://github.com/microsoft/PowerToys/blob/main/src/modules/fancyzones/FancyZonesLib/LayoutConfigurator.cpp). Changes to PowerToys' file formats or calculations may require updates here.
- It tells a title bar Windows draws from one an app draws itself by comparing where the window's content starts with where the window starts. For apps that draw their own bar it asks the window what's near the top (`WM_NCHITTEST`, the same question Windows asks to know where a window can be dragged). The answers show where the bar ends and what's in it: a plain title bar answers "caption" across its whole width, while tabs and buttons answer "content".
- It changes windows with `SetWindowLongPtr` and `SetWindowPos`, listens for window events with an out-of-context `SetWinEventHook`, and uses `RegisterHotKey` for its shortcut.
- A title bar it can't remove is clipped off: the window is placed so its content covers the zone, and a window region (`SetWindowRgn`) leaves the rest undrawn and lets clicks through. Windows draws its own frame and backdrop on top of any region, so those are switched off for the window meanwhile.
- It marks each window it changes with a property of its own, so even after an unexpected exit the next start recognizes those windows and can restore them.

FancyZones only resizes windows that have a resize border. Once the border is gone FancyZones moves the window on layout changes, and FancyBorderless takes care of the size.

## Event handling

FancyBorderless waits for window messages, file notifications and timers. System-wide hooks watch windows appearing or being destroyed and the end of a mouse drag. Location-change hooks are limited to processes whose windows it manages. A safety scan runs every five seconds. Memory and CPU use depend on the apps and activity being monitored.

## Settings file

Everything except the hotkey is in the tray menu. For manual tweaks, the file is at `%APPDATA%\FancyBorderless\config.json`. Window preferences and the hotkey reload on the next scan, normally within five seconds. Invalid JSON or an invalid hotkey keeps the previous settings; on first launch, defaults are used instead. Administrator mode edited in the file takes effect on the next launch.

| Setting | Default | Meaning |
|---|---|---|
| `removeTitleBars` | `true` | Same as the tray menu item. |
| `keepTitleBarApps` | `[]` | Apps that keep their title bar, by exe name (`MyApp.exe`; the `.exe` is optional). |
| `removeTitleBarApps` | `[]` | Apps that lose their title bar even when it has tabs or buttons in it. The hotkey and the tray menu edit both lists. |
| `toggleTitleBarHotkey` | `Ctrl+Alt+Shift+T` | Modifiers plus a letter, digit, F1-F24, Numpad0-9 or a named key like `PageUp`. Empty disables it. |
| `runAsAdministrator` | `false` | Request administrator rights on launch. The tray item can request them immediately; disabling it requires a restart to drop existing rights. |

The log at `%APPDATA%\FancyBorderless\FancyBorderless.log` records every window FancyBorderless changes and why.

## Command line

```powershell
.\FancyBorderless.exe --list | Out-Host      # monitors, calculated zones and snapped windows
.\FancyBorderless.exe --quit | Out-Host      # stop the running instance
.\FancyBorderless.exe --version | Out-Host
```

`--list` gives a quick overview of what FancyBorderless sees: the zones it calculated and the kind of title bar it found on each snapped window.

The pipeline makes PowerShell wait for this GUI executable and display its output.

`--quit` exits with status 1 when no instance is running. An unknown option prints usage and exits with status 2. `--list` reports missing FancyZones data but still exits with status 0.

## Good to know

- For square corners on Windows 11, turn on **Disable round corners when window is snapped** in FancyZones' settings. Snapped windows then meet the zone edges exactly.
- It never touches game memory or injects code.
- Tests cover canvas calculations at several DPI values, but this does not establish compatibility with every app or mixed-DPI monitor setup. Include display scaling in bug reports.
- Apps can refuse window changes. Repeated changes are limited, and hung windows are skipped during restoration.

## Build from source

Build on 64-bit Windows with Go 1.23.12 or newer; use a currently supported Go release. The application has no external Go modules and does not require cgo to build. Checked-in resources cover amd64 and arm64.

```powershell
go build -ldflags "-H=windowsgui -s -w" -o FancyBorderless.exe .
```

The icon comes with the source as `rsrc_windows_*.syso`, built from `winres/` with [go-winres](https://github.com/tc-hib/go-winres).

For the local maintenance gate, also put GNU make and a MinGW-w64 GCC toolchain on `PATH`. Race tests require cgo and GCC even though the application build does not. Run on Windows because the tests use native window APIs and hidden test windows:

```powershell
make lint test build
go tool cover '-func=coverage.out'
```

The make targets run `go vet`, run tests with `-race` and coverage, then build the executable. The single CI workflow runs these same targets on push or manual dispatch. GitHub Actions billing can prevent remote execution; use the local gate in that case.

## Uninstall

1. Untick **Start with Windows** in the tray menu. In administrator mode, do it while FancyBorderless runs as administrator, because removing its sign-in task needs those rights.
2. Click **Exit** to restore managed windows. An unresponsive app may need to be restarted if its title bar cannot be restored.
3. Delete `FancyBorderless.exe` and the folder `%APPDATA%\FancyBorderless`.

## Contributing

Bug reports, ideas and pull requests are welcome. If a window doesn't behave as expected, include the output of the `--list` command above, relevant log lines, display scaling and the PowerToys version. Run the local maintenance gate above before submitting code changes.

## License

MIT. See [LICENSE](LICENSE).

FancyBorderless is an independent project and isn't affiliated with or endorsed by Microsoft. FancyZones and PowerToys are Microsoft's.

## Disclaimer

AI was used for parts of the Code in this project.
