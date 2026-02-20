# eblitui-standalone

Full-featured desktop application UI for eblitui emulator cores. Built
on [Ebiten](https://ebitengine.org) and
[ebitenui](https://github.com/ebitenui/ebitenui), it provides a
complete standalone experience with a game library, settings, shader
effects, save states, rewind, RetroAchievements, and gamepad navigation.

Build tag: `!libretro` (excluded from libretro builds).


## Usage

```go
package main

import (
    "log"

    emucore "github.com/user-none/eblitui/api"
    "github.com/user-none/eblitui/standalone"
)

func main() {
    var factory emucore.CoreFactory = myFactory()
    if err := standalone.Run(factory); err != nil {
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
| `StateDetail` | Detail | Game info, artwork, play/resume, achievement progress |
| `StateSettings` | Settings | Tabbed configuration (Library, Appearance, Video, Audio, Rewind, RetroAchievements) |
| `StateScanProgress` | Scan Progress | ROM scanning with discovery and artwork phases |
| `StateError` | Error | Startup error handling (corrupted config recovery) |
| `StatePlaying` | Gameplay | Active emulation with pause menu overlay |


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

Requires the core to implement `MemoryInspector`.

### Themes

Supports a variety of built-in themes. Font size is configurable
(10-32pt).

### Settings

Organized in tabbed sections:

- **Library** - Scan directories, add/remove folders, rescan
- **Appearance** - Theme, font size
- **Video** - Shader effects for UI and gameplay
- **Audio** - Volume, mute, fast-forward mute
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
    metadata/        - RDB databases and artwork index
    artwork/         - Game artwork images
    saves/           - SRAM and save state files
    screenshots/     - Screenshot captures
```

All JSON writes are atomic (write to temp, rename) to prevent
corruption.


## Sub-packages

| Package | Description |
|---|---|
| `screens` | UI screens (library, detail, settings, scan, error) |
| `storage` | Config and library persistence with validation |
| `shader` | Shader registry, compilation, and effect pipeline |
| `style` | Themes, DPI-aware layout constants, widget builders |
| `types` | Shared interfaces for cross-package use |
| `achievements` | RetroAchievements manager and unlock sounds |
| `rdb` | RetroArch database (RDB) binary parser |


## Dependencies

Key external dependencies:

- `github.com/hajimehoshi/ebiten/v2` - Game engine and rendering
- `github.com/ebitenui/ebitenui` - UI widget framework
- `github.com/ebitengine/oto/v3` - Audio output
- `github.com/sqweek/dialog` - Native file dialogs
- `github.com/user-none/eblitui/api` - Core interfaces
- `github.com/user-none/eblitui/romloader` - ROM loading
- `github.com/user-none/go-rcheevos` - RetroAchievements client
- `golang.design/x/clipboard` - Clipboard access
- `golang.org/x/image` - Font rendering


## Testing

```
go test ./...
```

Test coverage includes state management, audio buffering, save states,
rewind, turbo, pause menu, achievement overlay, search, scanning,
storage validation, shader ordering, themes, and RDB parsing.
