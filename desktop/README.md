# eblitui-desktop

Full-featured desktop application UI for eblitui emulator cores. Built
on [Ebiten](https://ebitengine.org) and
[ebitenui](https://github.com/ebitenui/ebitenui), it provides a
complete desktop experience with a game library, settings, shader
effects, save states, rewind, cheats, RetroAchievements, and gamepad
navigation.

## Usage

```go
package main

import (
    "log"

    "github.com/user-none/eblitui/coreif"
    "github.com/user-none/eblitui/desktop"
)

func main() {
    var factory coreif.CoreFactory = myFactory()
    if err := desktop.Run(factory); err != nil {
        log.Fatal(err)
    }
}
```

`Run` is the single entry point. It initializes storage, configures the
Ebiten window, and starts the application loop. Everything is driven
from the `CoreFactory` passed in.


## Application States

The app is a state machine that transitions between screens:

| State | Screen | Description |
|---|---|---|
| `StateLibrary` | Library | Game grid/list with artwork, search, and sorting |
| `StateDetail` | Detail | Artwork and page sidebar: Overview (game info, discs, achievement progress), Cheats, Settings, and Screenshots |
| `StateSettings` | Settings | Tabbed configuration (Library, Appearance, Video, Audio, Rewind, RetroAchievements) |
| `StateScanProgress` | Scan Progress | ROM scanning with discovery, artwork, rumble, and cheat phases |
| `StateError` | Error | Startup error handling (corrupted config recovery) |
| `StatePlaying` | Gameplay | Active emulation with pause menu overlay |
| `StateFilePicker` | File Picker | Full-screen file and folder selection |


## Features

### Game Library

- Scan directories for ROMs with CRC32 hashing
- Metadata matching via RetroArch RDB databases (auto-downloaded)
- Artwork downloading from libretro thumbnail repositories
- Grid (icon) and list view modes
- Sort by title, last played, or play time
- Favorites filter
- Search overlay with keyboard filter

### Gameplay

- Dedicated emulation goroutine with audio-driven timing (ADT)
- Double-buffered framebuffer for thread-safe rendering
- Aspect-ratio-correct scaling with optional display cropping
- Keyboard input: WASD for D-pad, JKL/UIO for buttons
- Gamepad input: standard layout with 2-player support
- Gamepad rumble/haptic feedback via RetroArch CHT rumble files
- Pause menu with resume, return to library, and exit options
- Play time tracking per game

### Save States

- 10 save slots per game
- Auto-save on exit, auto-resume on launch
- SRAM battery save persistence
- Resume state (separate from manual slots)

### Rewind

- Configurable buffer size and frame step
- Hold-to-rewind with acceleration curve
- Requires `SaveStater` interface from the core

### Fast Forward

- 1x / 2x / 3x speed toggle
- Audio averaging for downsampled playback
- Optional mute during fast forward

### Rumble

Gamepad rumble support using RetroArch CHT rumble files. Per-game rumble
definitions are downloaded automatically during library scans from the
libretro-database repository.

CHT files define memory addresses to monitor and conditions that trigger
haptic feedback (value changes, increases, decreases, comparisons).
The rumble engine evaluates these conditions each frame and fires
vibration events via CoreHaptics on macOS.

Rumble strength is configurable with multiple levels:

| Level | Intensity | Duration |
|---|---|---|
| Off | - | - |
| 1x | 1x (min 40%) | 1x (min 250ms) |
| 2x | 2x | 2x (min 250ms) |
| 3x | 3x | 3x (min 250ms) |
| 4x | 4x | 2x (min 250ms) |
| Max | 100% | 2x (min 250ms) |

Requires the core to implement `coreif.Memory`.

**Note**
The number of games with rumble support is severely limited. Very few games
have rumble files and the number of events that trigger a rumble is also
sparse. The intensity and duration of rumble can be inconsistent between games.

### Cheats

The detail screen's Cheats page lists a game's cheats with an On/Off
toggle each. Cheats are applied between emulated frames through the
core's memory interface, so they take effect without changing the core.

Database cheats come from RetroArch CHT cheat files, downloaded during
library scans from the libretro-database repository for games matched
in the RDB. Both code string entries (such as Action Replay codes) and
RetroArch format entries are decoded. Master codes, entries that cannot
be decoded, and entries that cannot be converted to the core's memory
byte order are hidden. Every cheat starts off; the file's own enable
values are ignored.

Custom cheats are added on the Cheats page with a name and a code
string. Codes can be pasted with several codes separated by spaces, "+",
or new lines. Duplicate codes are rejected, and a custom cheat whose code
no longer parses shows why in place of its code.

Cheat support is available when the core implements `coreif.Memory` and
the `cheats` package knows the system's code formats. Currently that is
the Sega Saturn.

### Per-Game Settings

The detail screen's Settings page overrides global settings for one game.
It lists the core options the core marks `PerGame` (Bool and Select
options) under the name of the settings page each is on, and the
preprocessing shader effects under Video. Each setting cycles through
Inherit, which uses the global setting, then the setting's values. Reset
All to Global removes every override of the game.

Overrides are stored in the game's settings file and apply when the game
launches.

### Shader Effects

Shader effects can be enabled independently for the UI and gameplay
contexts. Multiple shaders can be active simultaneously and are applied
in weight-based order.

Preprocessing effects:
- **xBR** - Pixel art edge smoothing
- **Phosphor Persistence** - CRT phosphor decay ghosting

Post-processing shaders:
- **CRT** - Curved screen with RGB separation and vignette
- **Scanlines** - Horizontal scanline effect
- **Phosphor Glow** - Bright pixel bloom
- **LCD Grid** - Visible pixel grid with RGB subpixels
- **Color Bleed** - Composite video color bleeding
- **Dot Matrix** - Circular CRT phosphor dots
- **NTSC** - NTSC composite signal artifacts
- **Rainbow** - Rainbow banding interference
- **Gamma** - Gamma correction
- **Halation** - CRT internal light scattering
- **RF Noise** - RF signal noise
- **Rolling Band** - CRT rolling band interference
- **VHS** - VHS tape distortion
- **Interlace** - Interlaced display simulation
- **Horizontal Soften** - Horizontal blur
- **Vertical Blur** - Vertical blur
- **Monochrome** - Grayscale conversion
- **Sepia** - Sepia tone

Shaders are written in Ebiten's Kage shading language.

### RetroAchievements

- Login with RetroAchievements account
- Achievement tracking and unlock notifications with sound
- Badge and image caching
- Auto-screenshot on achievement unlock
- Encore mode (re-trigger unlocked achievements)
- Spectator mode (track without submitting)
- In-game achievement overlay
- Per-game and library-wide progress tracking

Requires the core to implement `coreif.Memory`.

### Themes

Supports a variety of built-in themes. The UI, font included, scales with
the window size so it stays readable from a desktop window up to a TV.

### Settings

Organized in tabbed sections:

- **Library** - Add/remove folders, scan, clean missing games
- **Appearance** - Theme
- **Video** - Shader effects for UI and gameplay
- **Audio** - Volume, mute, fast-forward mute
- **Input** - Button bindings, rumble level
- **Rewind** - Enable/disable, buffer size, frame step
- **RetroAchievements** - Login, notification preferences, modes


## Data Storage

Application data is stored per-emulator using the core's `DataDirName`:

| Platform | Path |
|---|---|
| macOS | `~/Library/Application Support/<DataDirName>/` |
| Linux | `~/.local/share/<DataDirName>/` (or `$XDG_DATA_HOME`) |
| Windows | `%APPDATA%/<DataDirName>/` |

Directory structure:

```
<DataDirName>/
    config.json      - Application settings
    library.json     - Game library and scan state
    removed.json     - User data of games removed from the library
    gamesettings/    - Per-game settings (save state slot, cheat choices, setting overrides)
    metadata/        - RDB databases and artwork index
    artwork/         - Game artwork images
    rumble/          - CHT rumble definition files
    cheats/          - CHT cheat files
    saves/           - SRAM and save state files
    screenshots/     - Screenshot captures
```

Per-game settings live in one JSON file per game under `gamesettings/`,
with a section per feature. The library file holds no per-game settings;
a version 1 library's save slots are moved there on first load.

Scanning never removes games. The detail screen offers Remove Disc when
the selected disc's file is missing and Remove from Library when every
file of the game is missing. Clean Library in settings does both for the
whole library. A removed game's entry, without its file locations and
added time, moves to `removed.json`, and a scan that finds the game again
restores its play time, favorite, and other user data. An unreadable
`removed.json` is treated as empty and overwritten on the next removal.

All JSON writes are atomic (write to temp, rename) to prevent
corruption.


## Sub-packages

| Package | Description |
|---|---|
| `screens` | UI screens (library, detail, settings, scan, error, file picker) |
| `screens/detail` | Detail screen pages: Overview, Cheats, Settings, and Screenshots |
| `screens/settings` | Settings screen sections |
| `cheats` | CHT cheat file and code string decoding into memory operations (standard library only) |
| `gamecheats` | A game's database and custom cheats, shared by the Cheats page and the cheat engine |
| `storage` | Config, library, and per-game settings persistence with validation |
| `widgets` | Shared ebitenui widget builders, the scroll view, sidebar, and multi-line text input |
| `widgets/textedit` | Text, caret, and selection model behind the multi-line text input |
| `shader` | Shader registry, compilation, and effect pipeline |
| `style` | Themes, DPI-aware layout constants, widget builders |
| `types` | Shared interfaces for cross-package use |
| `achievements` | RetroAchievements manager and unlock sounds |
| `filebrowser` | Directory listing, path input parsing and sidebar places for the file picker |
| `scanner` | Background ROM scanning and artwork, rumble, and cheat file downloads |
| `metadata` | RDB downloads and lookups for the core's metadata variants |
| `netutil` | HTTP download helpers |
| `display` | Aspect ratio sizing, scaling, and DPI helpers for the game image |


## Dependencies

Key external dependencies:

- `github.com/hajimehoshi/ebiten/v2` - Game engine and rendering
- `github.com/ebitenui/ebitenui` - UI widget framework
- `github.com/ebitengine/oto/v3` - Audio output
- `github.com/user-none/eblitui/coreif` - Core interfaces
- `github.com/user-none/eblitui/romloader` - ROM loading
- `github.com/user-none/eblitui/rdb` - RetroArch database (RDB) binary parser
- `github.com/user-none/go-rcheevos` - RetroAchievements client
- `golang.design/x/clipboard` - Clipboard access
- `golang.org/x/image` - Font rendering
- `golang.org/x/sys` - Windows drive and known folder lookup for the file picker


## Testing

```
go test ./...
```

Test coverage includes state management, audio buffering, save states,
rewind, turbo, pause menu, achievement overlay, search, scanning,
storage validation, shader ordering, themes, rumble engine, cheat
decoding and the cheat engine, the Cheats page, the text edit model, and
RDB parsing.
