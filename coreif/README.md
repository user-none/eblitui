# eblitui-coreif

Shared core interfaces for the eblitui emulator UI framework. This module
defines the contract between emulator cores and UI implementations, allowing
each to be developed independently.

Emulator cores implement these interfaces to describe their capabilities.
UI implementations consume them to drive rendering, audio, input, save
management, and settings without knowing the details of any specific system.

## Package

```
package coreif
```

```
import "github.com/user-none/eblitui/coreif"
```

## Interfaces

### CoreFactory

Entry point for the UI. Provides system metadata and creates emulator
instances.

- `SystemInfo() SystemInfo` - Returns system metadata used by the UI to
  configure screens, input mapping, settings menus, and data paths.
- `CreateEmulator() Emulator` - Creates a new emulator instance.
  Content is provided afterwards via `Emulator.SetRom` (cartridge) or
  `Emulator.SetDisc` (disc). Video standard detection is handled internally
  by the core.

### Emulator (required)

The core interface every emulator adapter must implement. Covers the per-frame
emulation loop: run a frame, read video and audio output, set input, and
manage region and timing.

| Method | Description |
|---|---|
| `RunFrame()` | Execute one frame of emulation |
| `GetFramebuffer() []byte` | Current frame as RGBA pixel data |
| `GetFramebufferStride() int` | Bytes per row in the framebuffer |
| `GetActiveHeight() int` | Current active display height in pixels |
| `GetAudioSamples() []int16` | Stereo 16-bit PCM audio samples for the frame |
| `SetInput(player int, buttons uint32)` | Set controller state as a button bitmask |
| `GetTiming() Timing` | FPS and scanline count for the current region |
| `SetOption(key string, value string)` | Apply a core option change by key |
| `SetRom(data []byte)` | Provide cartridge ROM data (disc cores ignore) |
| `SetDisc(disc DiscReader)` | Provide a streaming disc reader (cartridge cores ignore) |
| `SetBIOS(key string, data []byte) error` | Provide BIOS data; returns an error if invalid for the key |

### AspectProvider (optional)

Lets a core supply its pixel aspect ratio per frame when it depends on
the video mode (e.g. consoles that switch horizontal resolution). When
implemented, the UI uses this instead of the static
`SystemInfo.PixelAspectRatio`. Must be cheap (cached; recomputed only on
a mode change).

- `PixelAspectRatio() float64` - The current pixel aspect ratio.

### SaveStater (optional)

Enables save states, rewind, and auto-save. Implement on the Emulator struct
to opt in.

- `Serialize() ([]byte, error)` - Capture the complete emulator state.
- `Deserialize(data []byte) error` - Restore from previously serialized data.
- `SerializeSize() int` - Size of a serialized state in bytes.

### BatterySaver (optional)

Enables SRAM persistence for battery-backed saves.

- `HasSRAM() bool` - Whether the loaded ROM uses battery-backed save.
- `GetSRAM() []byte` - Copy of the current SRAM contents.
- `SetSRAM(data []byte)` - Load SRAM contents into the emulator.

### Memory (optional)

Canonical native bus address based access to the console's emulated
RAM. The region table is the access boundary: only the listed
canonical ranges are reachable, and accesses outside them transfer
nothing. Bus decode detail (mirrors, CPU partitions) is the core's
alone and never surfaces here. Calls happen between `RunFrame`
invocations.

- `ReadMemory(addr uint32, buf []byte) uint32` - Read from a native bus
  address into buf. Returns the number of bytes actually read.
- `WriteMemory(addr uint32, data []byte) uint32` - Write data to a
  native bus address. Returns the number of bytes actually written.
- `Regions() []BusRegion` - The accessible bus regions in canonical
  addresses. Static per machine.
- `ReadMemoryFlat(off uint32, buf []byte) uint32` - Read from the
  console's flat memory convention (the RetroAchievements layout).
  Reads are contiguous across region boundaries within the flat
  space. The layout is the core's alone; consumers needing the flat
  view (RetroAchievements, cht files) read through this and hold no
  layout knowledge.
- `WriteMemoryFlat(off uint32, data []byte) uint32` - Write data to
  the console's flat memory convention, matching `ReadMemoryFlat`'s
  layout and boundary behavior.

`BusRegion` carries the region's name, canonical native start, and
size.

### DiscReader

A streaming reader over a CD/disc image, passed to `Emulator.SetDisc` for
disc-based cores. Every signature uses only stdlib types, so a concrete
reader satisfies it structurally without importing this package.

- `ReadSector(lba int) ([]byte, error)` - Raw 2352-byte sector at the LBA.
- `NumTracks() int` - Number of tracks on the disc.
- `Track(i int) (number int, typ string, frames int, pregap int, startLBA int, control uint8)`
  - TOC fields for track index i in [0, NumTracks).
- `Close() error` - Release the underlying resources.

### DiscIdentifier (optional)

An optional interface a `CoreFactory` may implement so the UI can derive a
disc's identifying information without instantiating an emulator. Used to
group multi-disc games and resolve metadata for disc-based systems.

- `DiscInfo(disc DiscReader) (info DiscInfo, ok bool)` - The disc's derived
  information and true when it can be read.

`DiscInfo` carries only disc-derived facts; it has no knowledge of any
external catalog serial conventions.

| Field | Type | Description |
|-------|------|-------------|
| `ProductNumber` | `string` | The disc's product number; identical across a game's discs, so also used as the library/grouping key. |
| `DiscNumber` | `int` | 1-based position of this disc within the game. |
| `DiscTotal` | `int` | Total number of discs the game spans. |
| `Title` | `string` | On-disc game title, used as a display-name fallback. |

## Types

### Timing

Frame rate and scanline configuration returned by `Emulator.GetTiming()`.

| Field | Type | Description |
|---|---|---|
| `FPS` | `int` | Frames per second |
| `Scanlines` | `int` | Scanlines per frame |

CPU clocks are core-internal and not exposed through this type.

### Button

Describes a system-specific button for input mapping.

| Field | Type | Description |
|---|---|---|
| `Name` | `string` | Display name (e.g. "A", "Start") |
| `ID` | `int` | Bit position in the uint32 bitmask |

D-pad directions always occupy bits 0-3 via the constants `ButtonUp`,
`ButtonDown`, `ButtonLeft`, and `ButtonRight`. System-specific buttons
start at bit 4.

### CoreOption

Describes a configurable core setting for use in settings menus.

| Field | Type | Description |
|---|---|---|
| `Key` | `string` | Unique identifier passed to `SetOption` |
| `Label` | `string` | UI display name |
| `Description` | `string` | Help text |
| `Type` | `CoreOptionType` | `CoreOptionBool`, `CoreOptionSelect`, or `CoreOptionRange` |
| `Default` | `string` | Default value |
| `Values` | `[]string` | Choices (Select type only) |
| `Min` | `int` | Minimum (Range type only) |
| `Max` | `int` | Maximum (Range type only) |
| `Step` | `int` | Step size (Range type only) |
| `Category` | `CoreOptionCategory` | Settings section: `CoreOptionCategoryAudio`, `CoreOptionCategoryVideo`, `CoreOptionCategoryInput`, `CoreOptionCategoryCore` |
| `PerGame` | `bool` | Whether the option can be overridden per game |

### SystemInfo

System metadata returned by `CoreFactory.SystemInfo()`. The UI uses this
to configure display, input, audio, settings, data paths, and
RetroAchievements integration.

| Field | Type | Description |
|---|---|---|
| `Name` | `string` | Emulator name (e.g. "emmd") |
| `ConsoleName` | `string` | Full console name (e.g. "Sega Genesis") |
| `Extensions` | `[]string` | Supported ROM file extensions |
| `ScreenWidth` | `int` | Native screen width in pixels |
| `MaxScreenHeight` | `int` | Maximum screen height in pixels |
| `AspectRatio` | `float64` | Display aspect ratio |
| `SampleRate` | `int` | Audio sample rate in Hz |
| `Buttons` | `[]Button` | System-specific buttons |
| `Players` | `int` | Number of supported players |
| `CoreOptions` | `[]CoreOption` | Configurable core settings |
| `RDBName` | `string` | RetroAchievements database name |
| `ThumbnailRepo` | `string` | Thumbnail repository name |
| `RumbleRepoDir` | `string` | Console directory in the rumble repository |
| `DataDirName` | `string` | Data directory name for saves and config |
| `ConsoleID` | `int` | Console identifier for RetroAchievements |
| `CoreName` | `string` | Core implementation name |
| `CoreVersion` | `string` | Core version string |
| `Disc` | `bool` | True if content is a disc image (provided via `SetDisc`) |

## Implementing a Core

A core implementation consists of two parts:

1. A factory that implements `CoreFactory` to provide system metadata and
   create emulator instances.
2. An emulator struct that implements `Emulator` and whichever optional
   interfaces the core supports.

Optional interfaces are detected at runtime via type assertion, so cores
only need to implement what they support.
