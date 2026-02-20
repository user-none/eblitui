# eblitui-libretro

Libretro core wrapper for eblitui. Bridges the `emucore` interfaces to
the [libretro API](https://www.libretro.com) so emulator cores can be
built as shared libraries loadable by any libretro-compatible frontend
(RetroArch, etc.).

An emulator core calls `RegisterFactory` during `init()` and the
resulting binary exports all required `retro_*` functions.


## Usage

```go
package main

import (
    emucore "github.com/user-none/eblitui/api"
    "github.com/user-none/eblitui/libretro"
)

func init() {
    libretro.RegisterFactory(myFactory, []libretro.RetropadMapping{
        {RetroID: libretro.JoypadA, BitID: 4},
        {RetroID: libretro.JoypadB, BitID: 5},
        {RetroID: libretro.JoypadStart, BitID: 6},
    })
}

func main() {}
```

The binary must be built as a C shared library:

```
go build -buildmode=c-shared -o mycore_libretro.so
```


## Public API

### RegisterFactory

```go
func RegisterFactory(f emucore.CoreFactory, mapping []RetropadMapping)
```

Sets the `CoreFactory` and joypad button mapping. Must be called during
`init()` before any `retro_*` function runs. The factory's `SystemInfo`
is read immediately and used to configure system metadata, core options,
and memory buffers.

### RetropadMapping

```go
type RetropadMapping struct {
    RetroID int // RETRO_DEVICE_ID_JOYPAD_* constant
    BitID   int // emucore bit position (from Button.ID)
}
```

Maps libretro joypad buttons to emucore button bit positions. D-pad
directions (Up/Down/Left/Right) are mapped automatically and do not
need entries.

### Joypad Constants

| Constant | libretro Button |
|---|---|
| `JoypadB` | B |
| `JoypadY` | Y |
| `JoypadSelect` | Select |
| `JoypadStart` | Start |
| `JoypadA` | A |
| `JoypadX` | X |
| `JoypadL` | L |
| `JoypadR` | R |
| `JoypadL2` | L2 |
| `JoypadR2` | R2 |
| `JoypadL3` | L3 |
| `JoypadR3` | R3 |


## Exported libretro Functions

All required libretro 1.0 functions are exported:

- System: `retro_api_version`, `retro_get_system_info`,
  `retro_get_system_av_info`, `retro_get_region`
- Lifecycle: `retro_init`, `retro_deinit`, `retro_load_game`,
  `retro_unload_game`, `retro_run`, `retro_reset`
- Callbacks: `retro_set_environment`, `retro_set_video_refresh`,
  `retro_set_audio_sample`, `retro_set_audio_sample_batch`,
  `retro_set_input_poll`, `retro_set_input_state`
- Save states: `retro_serialize`, `retro_unserialize`,
  `retro_serialize_size`
- Memory: `retro_get_memory_data`, `retro_get_memory_size`
- Input: `retro_set_controller_port_device`
- Cheats: `retro_cheat_reset`, `retro_cheat_set`
  (`retro_load_game_special` is a no-op)


## Core Options

The following options are registered with the frontend automatically:

- **Region** (Auto / NTSC / PAL) - Video region override. Auto uses
  the region detected from the ROM.

Any additional options from `SystemInfo.CoreOptions` are also registered
using the core's short name as a key prefix.


## Optional Interface Support

The wrapper detects optional emucore interfaces via type assertion when
a game is loaded:

| Interface | libretro Feature |
|---|---|
| `SaveStater` | Save states (`retro_serialize` / `retro_unserialize`) |
| `MemoryMapper` | Memory regions (`retro_get_memory_data` / `retro_get_memory_size`) |


## Pixel Format

Video output uses XRGB8888. The wrapper converts from the emucore
RGBA framebuffer to XRGB8888 for the frontend.


## Dependencies

- `github.com/user-none/eblitui/api`

No other external dependencies. The C headers (`libretro.h`, `cfuncs.h`)
are included in the package.


## Testing

```
go test ./...
```

Tests cover pixel format conversion, retropad constant validation,
and helper utilities.
