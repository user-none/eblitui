# eblitui-api

Shared API interfaces for the eblitui emulator UI framework. This module
defines the contract between emulator cores and UI implementations, allowing
each to be developed independently.

Emulator cores implement these interfaces to describe their capabilities.
UI implementations consume them to drive rendering, audio, input, save
management, and settings without knowing the details of any specific system.

## Package

```
package emucore
```

```
import "github.com/user-none/eblitui/api"
```

## Interfaces

### CoreFactory

Entry point for the UI. Provides system metadata and creates emulator
instances.

- `SystemInfo() SystemInfo` - Returns system metadata used by the UI to
  configure screens, input mapping, settings menus, and data paths.
- `CreateEmulator(rom []byte, region Region) (Emulator, error)` - Creates
  a new emulator instance from ROM data and a video region.
- `DetectRegion(rom []byte) (Region, bool)` - Auto-detects the region from
  ROM data. The bool indicates whether the region was found in a database
  versus falling back to a default.

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
| `GetRegion() Region` | Current video region |
| `SetRegion(region Region)` | Change the video region |
| `GetTiming() Timing` | FPS and scanline count for the current region |
| `SetOption(key string, value string)` | Apply a core option change by key |

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

### MemoryInspector (optional)

Flat address-based memory reads. Used by RetroAchievements to inspect
emulator memory without knowing the internal memory map.

- `ReadMemory(addr uint32, buf []byte) uint32` - Read from a flat address
  into buf. Returns the number of bytes actually read. The core adapter
  is responsible for mapping flat addresses to internal memory regions.

### MemoryMapper (optional)

Named memory region access following the libretro memory model.

- `MemoryMap() []MemoryRegion` - List available memory regions with sizes.
- `ReadRegion(regionType int) []byte` - Read a copy of the specified region.
- `WriteRegion(regionType int, data []byte)` - Write to the specified region.

Region type constants:

| Constant | Description |
|---|---|
| `MemorySaveRAM` | Battery-backed save RAM (RETRO\_MEMORY\_SAVE\_RAM) |
| `MemorySystemRAM` | Main system RAM (RETRO\_MEMORY\_SYSTEM\_RAM) |

## Types

### Region

Video region enumeration. Affects frame rate and scanline count.

| Value | Description |
|---|---|
| `RegionNTSC` | NTSC (60 Hz) |
| `RegionPAL` | PAL (50 Hz) |

Implements `String()` returning `"NTSC"`, `"PAL"`, or `"Unknown"`.

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
| `DataDirName` | `string` | Data directory name for saves and config |
| `ConsoleID` | `int` | Console identifier for RetroAchievements |
| `CoreName` | `string` | Core implementation name |
| `CoreVersion` | `string` | Core version string |

## Implementing a Core

A core implementation consists of two parts:

1. A factory that implements `CoreFactory` to provide system metadata and
   create emulator instances.
2. An emulator struct that implements `Emulator` and whichever optional
   interfaces the core supports.

Optional interfaces are detected at runtime via type assertion, so cores
only need to implement what they support.
