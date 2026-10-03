# FancyBorderless

Removes the title bar from windows that [FancyZones](https://learn.microsoft.com/windows/powertoys/fancyzones) has snapped, so they fill their zone exactly. Games included.

I built this for a 49" 32:9 monitor where I split up stuff. FancyZones handles the layouts well, but every snapped window keeps its title bar. For most apps that only wastes space. For a game running in windowed mode at exactly the zone size it's worse: the title bar pushes the picture down and the bottom of the game ends up cut off.

## What it does

- When FancyZones snaps a window, FancyBorderless removes its title bar and border and fits it to the zone.
- **Resizable windows** (Plex, editors, most apps) are resized to fill the zone, including after you switch FancyZones layouts.
- **Fixed-size windows** (most games in windowed mode) keep their size. Most games keep drawing at their own resolution when the window shrinks, so making them smaller would cut the picture off. A game whose resolution is within 64 px of the zone size gets the full zone. That covers the pixels the title bar and Windows' maximum window height take away from a game running at exactly the zone size.
- **Programs that put their title bar back** (Total War: Warhammer III does this within a fraction of a second) aren't fought over. FancyBorderless leaves the frame on and places the window so the picture covers the zone and the title bar sits above the top edge of the screen.
- **Apps that draw their own plain title bar** (many WPF and UWP apps, SFVIP Player for example) get it hidden above the screen the same way.
- **Browsers, Explorer, Windows Terminal** and other apps that put tabs or controls into their title bar are left alone, without any list to maintain. There's nothing to remove there.
- When a window leaves its zone, it gets its title bar back. Turning FancyBorderless off or exiting it restores every window it changed.

## How it works

FancyBorderless works entirely from the outside. It doesn't inject anything into other processes and doesn't hook games.

- FancyZones marks every window it snaps with a window property (`FancyZones_zones`) holding the zone number. FancyBorderless reads that property to know which windows are snapped and where.
- It reads FancyZones' layout files (`applied-layouts.json`, `custom-layouts.json`) and calculates the zone rectangles with the same integer math FancyZones uses, for custom layouts and the built-in templates alike, so windows land on the same pixels.
- To tell a title bar Windows draws from one an app draws itself, it compares where the window's content starts with where the window starts. For apps that draw their own bar it asks the window what's at each point near the top (`WM_NCHITTEST`, the same question Windows asks to know where a window can be dragged). A plain title bar answers "caption" across its whole width; a tab strip doesn't.
- It changes windows with the standard `SetWindowLongPtr` and `SetWindowPos` calls, listens for window events with an out-of-context `SetWinEventHook`, and uses `RegisterHotKey` for its shortcut.
- It marks each window it changes with a window property of its own. If FancyBorderless is killed, the next instance finds those marks and can still put the title bars back.

FancyZones only resizes windows that have a resize border. Once the border is gone FancyZones just moves the window on layout changes, which is why FancyBorderless does the resizing itself.

## Resource use

FancyBorderless sleeps until something happens. It listens system-wide only for windows appearing and for the end of a mouse drag, follows window moves only for the programs whose windows it manages, and gets notified by Windows when FancyZones' files change. Mouse movement never wakes it up. Apart from that there's one quick check every five seconds.

Measured about 14 MB of memory and 0.05% of one CPU core while idle, which is at the limit of what Windows can measure. Moving the mouse costs nothing.

## Requirements

- Windows 10 or 11, 64-bit
- [PowerToys](https://github.com/microsoft/PowerToys) with FancyZones enabled. Custom layouts and the built-in templates (Columns, Rows, Grid, Priority Grid, Focus) both work.

## Install

1. Download `FancyBorderless.exe` from the [latest release](https://github.com/RamazanKara/FancyBorderless/releases/latest).
2. Put it anywhere and run it. It sits in the system tray.
3. The exe isn't code-signed, so Windows SmartScreen may warn the first time. Choose "More info", then "Run anyway", or build it yourself (see below).
4. Click the tray icon and tick **Start with Windows** if you want it to run at login.

## Use

Snap windows with FancyZones as usual: Shift-drag, Win+arrow keys or your layout hotkeys. FancyBorderless follows along.

**Ctrl+Alt+Shift+T** removes or brings back the title bar of the window in front. FancyBorderless remembers the choice for that app, so its other windows and future ones follow it. A short notification tells you what happened.

Click the tray icon for the settings:

- Remove title bars (turns everything on or off)
- Square corners (turns off Windows 11's rounded corners on borderless windows so they meet the zone edges)
- Start with Windows
- Run as administrator (needed for games that run as administrator, see below)
- Keep title bar for (every app that's snapped or that you've excluded; tick an app to keep its title bar)
- Open settings file, Open log, Exit

### Games

- Set the game to **Windowed** mode at the size of its zone, for example 2560×1440 for half of a 5120×1440 screen.
- Snap it into its zone once. FancyZones remembers the zone and puts the game back there on every launch ("Move newly created windows to their last known zone" in FancyZones' settings).
- Don't add games to FancyZones' excluded apps, since FancyBorderless only handles windows that FancyZones snaps.
- Borderless and exclusive fullscreen modes don't work, because FancyZones doesn't snap those windows.

### Games that run as administrator

Some games run as administrator, often because of their anti-cheat (Marvel Rivals does). Windows doesn't let normal programs move or change the windows of administrator programs, so neither FancyZones nor FancyBorderless can touch them at first. If you press the hotkey on such a game, FancyBorderless tells you so.

To make them work:

1. In PowerToys Settings, General tab, turn on **Always run as administrator**. FancyZones needs it to snap the game ([PowerToys docs](https://learn.microsoft.com/windows/powertoys/administrator)).
2. In FancyBorderless's tray menu, tick **Run as administrator** and confirm the Windows prompt. FancyBorderless restarts with administrator rights.

With both on, **Start with Windows** uses a scheduled task, the same way PowerToys does it, so there's no prompt at every sign-in.

## Settings file

Everything except the hotkey is in the tray menu. The file is at `%APPDATA%\FancyBorderless\config.json` and changes apply as soon as you save it.

| Setting | Default | Meaning |
|---|---|---|
| `removeTitleBars` | `true` | Same as the tray menu item. |
| `squareCorners` | `true` | Same as the tray menu item. |
| `keepTitleBarApps` | `[]` | Apps that keep their title bar, by exe name (`Plex.exe`; the `.exe` is optional). The hotkey and the tray menu edit this list. |
| `toggleTitleBarHotkey` | `Ctrl+Alt+Shift+T` | Modifiers plus a letter, digit, F1-F24, Numpad0-9 or a named key like `PageUp`. Empty disables it. |
| `runAsAdministrator` | `false` | Same as the tray menu item. |

The log at `%APPDATA%\FancyBorderless\FancyBorderless.log` lists every window FancyBorderless changes and why.

## Command line

```
FancyBorderless.exe --list      monitors, calculated zones, snapped windows and their title bars
FancyBorderless.exe --quit      stop the running instance
FancyBorderless.exe --version
```

`--list` is the first thing to run if a window doesn't do what you expect. It shows the zones FancyBorderless calculated and what kind of title bar it found on each snapped window.

## Limitations

- Hiding a title bar above the screen only works in zones along the top of a monitor. In other zones a program that insists on its title bar, or draws its own, keeps it.
- Windows that run as administrator need PowerToys and FancyBorderless in administrator mode (see "Games that run as administrator").
- If FancyBorderless is killed, windows stay borderless until it starts again. It then recognizes them and restores their title bars when they leave their zone or when it exits.
- It never touches game memory or injects code.
- So far it's been tested at 100% display scaling.

## Build from source

Needs Go 1.23 or newer. No other dependencies, no cgo.

```
go build -ldflags "-H=windowsgui -s -w" -o FancyBorderless.exe .
```

## License

MIT. See [LICENSE](LICENSE).

FancyBorderless is an independent project and isn't affiliated with or endorsed by Microsoft. FancyZones and PowerToys are Microsoft's.
