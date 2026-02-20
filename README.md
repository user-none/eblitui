# eblitui

Shared UI framework for retro emulators. Provides common interfaces,
front ends, and utilities so multiple emulator cores can share the same
UI code while remaining fully independent applications.

Each emulator links the components it needs and ships as its own binary.
This is not a multi-system emulator -- it is a shared UI layer that
individual emulators build on top of.


## Components

### api

Shared interface contract between emulator cores and UI implementations.
Defines the `CoreFactory`, `Emulator`, and optional capability interfaces
(`SaveStater`, `BatterySaver`, `MemoryInspector`, `MemoryMapper`)
that allow cores and UIs to work together without
knowing each other's internals.

Zero external dependencies.

### standalone

Full-featured desktop application UI built on [Ebiten](https://ebitengine.org).
Includes a game library with scanning and artwork, settings, save states,
rewind, shader effects, RetroAchievements integration, gamepad and
keyboard navigation, and audio-driven timing.

### libretro

Libretro core wrapper that bridges the emucore interfaces to the
[libretro API](https://www.libretro.com). Builds as a shared library
(`.so`/`.dylib`/`.dll`) loadable by any libretro-compatible frontend
such as RetroArch.

### rdb

Parser for RetroArch/libretro RDB files. These are MessagePack-encoded
binary databases containing game metadata (name, developer, publisher,
genre, CRC32, MD5, serial, release date, etc.). Provides fast lookups
by CRC32 and MD5 for identifying ROMs and retrieving their metadata.

Zero external dependencies.

### romloader

ROM loading utility that handles raw files and compressed archives
(ZIP, 7z, gzip/tar.gz, RAR). Auto-detects formats via magic bytes
and extracts ROMs by extension.


## How It Works

An emulator core implements the interfaces defined in `api`. The core
then links one of the UI packages (`standalone` or `libretro`)
and provides its `CoreFactory` as the entry point. The UI handles
everything else: rendering, audio, input, saves, settings, and any
additional features.

```
Emulator Core (e.g. emkiii, emmd)
    |
    implements emucore interfaces (api)
    |
    +---> standalone   (desktop app)
    +---> libretro     (shared library for RetroArch etc.)
```

Optional interfaces are detected at runtime via type assertion. A core
only implements what it supports and the UI adapts accordingly.


## Module Structure

This is a Go workspace. Each component is its own Go module:

| Module | Import Path |
|---|---|
| api | `github.com/user-none/eblitui/api` |
| standalone | `github.com/user-none/eblitui/standalone` |
| libretro | `github.com/user-none/eblitui/libretro` |
| rdb | `github.com/user-none/eblitui/rdb` |
| romloader | `github.com/user-none/eblitui/romloader` |


## Building

Individual emulators are built from their own repositories by importing
the eblitui modules they need. There is no single binary built from
this repository directly.

Tests for each component can be run from their directories:

```
go test ./api/...
go test ./romloader/...
go test ./standalone/...
go test ./libretro/...
go test ./rdb/...
```
