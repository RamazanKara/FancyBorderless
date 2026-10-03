# FancyBorderless

Removes the title bar and border from windows that [FancyZones](https://learn.microsoft.com/windows/powertoys/fancyzones) has snapped, so they fill their zone exactly. Games included.

I built this for a 49" 32:9 monitor where the left half runs a game and the right half holds Plex, an editor or a browser. FancyZones handles the layouts well, but every snapped window keeps its title bar. For most apps that only wastes space. For a game running in windowed mode at exactly the zone size it's worse: the title bar pushes the picture down and the bottom of the game ends up cut off.

## What it does

- When FancyZones snaps a window, FancyBorderless removes its title bar and border and fits it to the zone.
- **Resizable windows** (Plex, editors, most apps) are resized to fill the zone, including after you switch FancyZones layouts.
- **Fixed-size windows** (most games in windowed mode) keep their size. Most games keep drawing at their own resolution when the window shrinks, so making them smaller would cut the picture off. A game whose resolution is within 64 px of the zone size gets the full zone. That covers the pixels the title bar and Windows' maximum window height take away from a game running at exactly the zone size.
- **Programs that put their title bar back** (Total War: Warhammer III does this within a fraction of a second) are not fought over. FancyBorderless leaves the frame on and places the window so the picture covers the zone and the title bar sits above the top edge of the screen.
- When a window leaves its zone, it gets its title bar back. Pausing or exiting FancyBorderless restores every window it changed.

## How it works

FancyBorderless works entirely from the outside. It doesn't inject anything into other processes and doesn't hook games.

- FancyZones marks every window it snaps with a window property (`FancyZones_zones`) holding the zone number. FancyBorderless reads that property to know which windows are snapped and where.
- It reads FancyZones' layout files (`applied-layouts.json`, `custom-layouts.json`) and calculates the zone rectangles with the same integer math FancyZones uses, so windows land on the same pixels.
- It changes windows with the standard `SetWindowLongPtr` and `SetWindowPos` calls, listens for window events with an out-of-context `SetWinEventHook`, and uses `RegisterHotKey` for its shortcut.

FancyZones only resizes windows that have a resize border. Once the border is gone FancyZones just moves the window on layout changes, which is why FancyBorderless does the resizing itself.

## Requirements

- Windows 10 or 11, 64-bit
- [PowerToys](https://github.com/microsoft/PowerToys) with FancyZones enabled
- Custom FancyZones layouts. The built-in templates (Columns, Rows, Grid, Priority Grid, Focus) aren't supported yet.

## Install

1. Download `FancyBorderless.exe` from the [latest release](https://github.com/RamazanKara/FancyBorderless/releases/latest).
2. Put it anywhere and run it. It sits in the system tray.
3. The exe isn't code-signed, so Windows SmartScreen may warn the first time. Choose "More info", then "Run anyway", or build it yourself (see below).
4. To start it with Windows, press Win+R, type `shell:startup` and put a shortcut to the exe in that folder.

## Use

Snap windows with FancyZones as usual: Shift-drag, Win+arrow keys or your layout hotkeys. FancyBorderless follows along.

For games:

- Set the game to **Windowed** mode at the size of its zone, for example 2560×1440 for half of a 5120×1440 screen.
- Snap it into its zone once. FancyZones remembers the zone and puts the game back there on every launch ("Move newly created windows to their last known zone" in FancyZones' settings).
- Don't add games to FancyZones' excluded apps, since FancyBorderless only handles windows that FancyZones snaps.
- Borderless and exclusive fullscreen modes don't work, because FancyZones doesn't snap those windows.

**Ctrl+Alt+Shift+T** shows or hides the title bar of the window in front. On a snapped window the choice lasts until the window closes.

Right-click the tray icon for: Paused, Reload settings and layouts, Open settings file, Open log, Exit.

## Settings

The settings file is created on first run at `%APPDATA%\FancyBorderless\config.json`. Changes apply as soon as you save it.

| Setting | Default | Meaning |
|---|---|---|
| `removeTitleBars` | `true` | Turns everything off without exiting. |
| `onlyApps` | `[]` | If not empty, only these programs lose their title bar. |
| `keepTitleBarApps` | Explorer, Firefox, Chrome, Edge, Brave, Windows Terminal | Never touched. These draw tabs or controls into their title bar. |
| `squareCorners` | `true` | Turns off Windows 11's rounded corners on borderless windows so they meet the zone edges. |
| `toggleTitleBarHotkey` | `Ctrl+Alt+Shift+T` | Modifiers plus a letter, digit, F1-F24, Numpad0-9 or a named key like `PageUp`. Empty disables it. |

Program names are exe file names, for example `Plex.exe`. The `.exe` is optional.

The log is at `%APPDATA%\FancyBorderless\FancyBorderless.log`. It lists every window FancyBorderless changes and why.

## Command line

```
FancyBorderless.exe --list      monitors, calculated zones and snapped windows
FancyBorderless.exe --quit      stop the running instance
FancyBorderless.exe --version
```

`--list` is the first thing to run if a window doesn't land where you expect. It shows the zones FancyBorderless calculated, so you can compare them with what FancyZones does.

## Limitations

- Only custom FancyZones layouts are supported.
- Hiding the title bar above the screen only works in zones along the top of a monitor. In other zones a program that insists on its title bar keeps it.
- Windows running as administrator can only be changed if FancyBorderless runs as administrator too. The log says when this happens.
- If FancyBorderless crashes, windows stay borderless until it runs again or you press the hotkey.
- It never touches game memory or injects code, but anti-cheat systems differ and I can't promise every one of them ignores window style changes.

## Build from source

Needs Go 1.23 or newer. No other dependencies, no cgo.

```
go build -ldflags "-H=windowsgui -s -w" -o FancyBorderless.exe .
```

## License

MIT. See [LICENSE](LICENSE).

FancyBorderless is an independent project and isn't affiliated with or endorsed by Microsoft. FancyZones and PowerToys are Microsoft's.
