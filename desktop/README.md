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
    emu := desktop.Emulator{
        Name:        "myemu",
        Version:     "1.0.0",
        DataDirName: "myemu",
    }
    if err := desktop.Run(factory, emu); err != nil {
        log.Fatal(err)
    }
}
```

`Run` is the single entry point. It initializes storage, configures the
Ebiten window, and starts the application loop. The `CoreFactory`
supplies the core: the emulation and the facts about it in
`coreif.SystemInfo`. The `Emulator` describes the application built
around the core:

| Field | Description |
|---|---|
| `Name`, `Version` | Window title, About, and the RetroAchievements client name |
| `DataDirName` | Data directory name (see Data Storage) |
| `Icon` | PNG, JPEG, or WebP icon for the window and About |
| `RetroAchievementsConsoleID` | The console's RetroAchievements ID; 0 turns achievements off |
| `CheatFormat` | The `cheats` system whose code formats the emulator's cheats use (e.g. `"saturn"`); empty turns cheats off |
| `Update` | Where releases are published, the running version, and the update signing public keys (`algorithm:base64`); nil or no keys turns update checks off |
| `Artwork` | The artwork site's HTTPS base URL and the public keys (`algorithm:base64`) trusted to sign its index; nil turns artwork downloads off |
| `Metadata` | Source of game metadata (`content.MetadataSource`); nil shows file names only |
| `Cheats` | Source of database cheats (`content.CheatsSource`); nil offers custom cheats only |
| `Rumble` | Source of .erumble rumble files (`content.RumbleSource`); nil turns rumble off |


## Application States

The app is a state machine that transitions between screens:

| State | Screen | Description |
|---|---|---|
| `StateLibrary` | Library | Game grid/list with artwork, search, and sorting |
| `StateDetail` | Detail | Box art over the page sidebar: Overview (game info, discs, achievement progress), Artwork (covers, discs, flyers and posters on disk), Cheats, Settings, and Screenshots |
| `StateSettings` | Settings | Tabbed configuration (Library, Appearance, Video, Audio, Rewind, RetroAchievements) |
| `StateScanProgress` | Scan Progress | ROM scanning with discovery and artwork phases |
| `StateError` | Error | Startup error handling (corrupted config recovery) |
| `StatePlaying` | Gameplay | Active emulation with pause menu overlay |
| `StateFilePicker` | File Picker | Full-screen file and folder selection |


## Features

### Game Library

- Scan directories for ROMs, keyed by CRC32 for cartridges and the disc's
  product number for disc games
- Game metadata (title, region, developer, and more) from the emulator's
  `Metadata` source, looked up when shown; the library stores none of it,
  and a game without metadata shows its file name
- Artwork downloaded during library scans from the emulator's artwork site
  (`Emulator.Artwork`): the site's `index.json` lists each game ID's images
  (front and back covers, discs, flyers, posters) with the values that tell
  images of one type apart (disc number, region, variant letter), and their
  size, CRC-32, and SHA-256. The index is used only when `index.sig` holds
  a valid signature by one of the artwork `Keys`; otherwise no artwork is
  downloaded. A scan downloads the images that are missing or whose size or
  CRC-32 differs from the index, and keeps a download only when its size
  and SHA-256 match. The Library setting "Artwork download" limits which
  images a scan downloads (front cover, covers, or all artwork); it never
  removes artwork already on disk. Images are stored under names built from
  the index values, such as `disc_2_eu.webp` or `flyer_b.webp`. Images
  larger than 4096x4096 are not decoded
- The library and the detail sidebar show the front cover: one without a
  region first, then USA, Japan, Europe, and any other region. The detail
  Artwork page shows every image on disk, in order front, back, discs,
  flyers, posters, by disc number, region and variant. A game without
  `boxart.webp` shows its older `boxart.png`, which Clean Library removes
  once `boxart.webp` is present
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
- Gamepad rumble/haptic feedback via .erumble rumble files
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

Gamepad rumble support using .erumble rumble files (see the `rumble`
module), supplied by the emulator's `Rumble` source and looked up by game
ID when a game starts. A disc game uses a file for the disc being played
(`<gameid>.disc<n>`) when the source has one, otherwise the game's shared
file. Rumble needs the core to expose memory.

Rumble files define memory addresses to monitor and the conditions that
trigger haptic feedback. The rumble engine evaluates these conditions
each frame and drives the gamepad motors.

Rumble intensity is set per controller profile as a multiplier from 0
(off) to 3x in 0.05 steps. The authored motor levels are multiplied by
it and clamped to the gamepad maximum.

Requires the core to implement `coreif.Memory`.

**Note**
The number of games with rumble support is severely limited. Very few games
have rumble files and the number of events that trigger a rumble is also
sparse. The intensity and duration of rumble can be inconsistent between games.

### Cheats

The detail screen's Cheats page lists a game's cheats with an On/Off
toggle each. Cheats are applied between emulated frames through the
core's memory interface, so they take effect without changing the core.

Database cheats come from the emulator's `Cheats` source, looked up by
the game's ID: a name and a code string per cheat, decoded the same way
as custom cheats. Cheats whose code cannot be decoded or converted to the
core's memory byte order are hidden, and the master code is not applied.
Every cheat starts off.

Custom cheats are added on the Cheats page with a name and a code
string. Codes can be pasted with several codes separated by spaces, "+",
or new lines. Duplicate codes are rejected, and a custom cheat whose code
no longer parses shows why in place of its code.

Cheat support is available when the core implements `coreif.Memory` and
the emulator's `CheatFormat` names a system the `cheats` package knows.
Currently that is `"saturn"` (Action Replay codes).

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

Application data is stored per-emulator using `Emulator.DataDirName`:

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
    artwork/         - Game artwork images, a directory per game (<type>[_<disc>][_<region>][_<variant>].webp)
    saves/           - SRAM and save state files
    screenshots/     - Screenshot captures
```

Per-game settings live in one JSON file per game under `gamesettings/`,
with a section per feature. The library file holds no per-game settings;
a version 1 library's save slots are moved there on first load. Library
entries hold a game's ID, files, and user data only; a version 2
library's metadata fields are dropped on first load.

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
| `cheats` | Cheat code string decoding into memory operations (standard library only) |
| `gamecheats` | A game's database and custom cheats, shared by the Cheats page and the cheat engine |
| `storage` | Config, library, and per-game settings persistence with validation |
| `widgets` | Shared ebitenui widget builders, the scroll view, sidebar, and multi-line text input |
| `widgets/textedit` | Text, caret, and selection model behind the multi-line text input |
| `shader` | Shader registry, compilation, and effect pipeline |
| `style` | Themes, DPI-aware layout constants, widget builders |
| `types` | Shared interfaces for cross-package use |
| `achievements` | RetroAchievements manager and unlock sounds |
| `filebrowser` | Directory listing, path input parsing and sidebar places for the file picker |
| `scanner` | Background ROM scanning and artwork downloads |
| `artwork` | The artwork index format, which artwork files to download, local artwork listing and captions, and box art lookup |
| `signing` | Signing key parsing and detached signature verification for the update manifest and the artwork index |
| `metadata` | The emulator's game metadata, and title and region resolution |
| `content` | The game metadata and cheat types, and the source interfaces an emulator supplies metadata, cheats, and rumble files through |
| `netutil` | HTTP download helpers |
| `display` | Aspect ratio sizing, scaling, and DPI helpers for the game image |


## Dependencies

Key external dependencies:

- `github.com/hajimehoshi/ebiten/v2` - Game engine and rendering
- `github.com/ebitenui/ebitenui` - UI widget framework
- `github.com/ebitengine/oto/v3` - Audio output
- `github.com/user-none/eblitui/coreif` - Core interfaces
- `github.com/user-none/eblitui/romloader` - ROM loading
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
game metadata resolution.
