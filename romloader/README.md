# eblitui-romloader

A shared ROM loading utility for eblitui UIs. Handles loading ROM files
from raw files and compressed archives (ZIP, 7z, gzip/tar.gz, RAR), and
streaming disc images for disc-based systems (CHD and cue/bin).

Valid ROM extensions are passed by the caller rather than being hardcoded.
Extensions come from `SystemInfo.Extensions` at the call site.


## Usage

```go
import "github.com/user-none/eblitui/romloader"

// Load a ROM, searching archives for files matching the given extensions
data, filename, err := romloader.Load(path, []string{".sms"})
```

The `Load` function auto-detects archive formats via magic bytes and
extracts the first file whose extension matches the provided list.
For non-archive files, the file is read directly if its extension matches.


## Public API

```go
// Load reads a ROM from a file path. It auto-detects compressed
// archives via magic bytes and extracts the first file matching
// one of the given extensions.
//
// Returns the ROM data, the filename (basename only), and any error.
func Load(path string, extensions []string) ([]byte, string, error)
```

### Errors

```go
var ErrNoROMFile         // no ROM file found in archive
var ErrUnsupportedFormat // unrecognized file format
var ErrFileTooLarge      // file exceeds 8MB safety limit
```


## Supported Formats

Detection uses magic bytes first (reliable), then falls back to
file extension.

| Format     | Magic bytes              | Extensions        |
|------------|--------------------------|-------------------|
| ZIP        | `PK\x03\x04`            | .zip              |
| 7z         | `7z\xBC\xAF\x27\x1C`   | .7z               |
| GZIP/TAR   | `\x1F\x8B`              | .gz, .tgz, .tar.gz |
| RAR        | `Rar!`                   | .rar              |
| Raw ROM    | (none)                   | Caller-provided   |

For archives, the loader searches for the first file whose extension
matches one of the provided ROM extensions (case-insensitive).
Directories and non-matching files are skipped.

For plain gzip files (not tar.gz), the decompressed content is
returned directly since the file is not a multi-file archive.


## Disc Images

For disc-based systems, `OpenDisc` opens a streaming reader over a CD image.
The backend is selected by file extension:

- `.chd` - a CHD (V5) disc image.
- `.cue` - a cue sheet referencing one or more raw `.bin` track files.

Any other extension returns `ErrUnsupportedDisc`. A bare `.bin` is not an
openable disc. It is only a component referenced by a cue sheet.

```go
disc, err := romloader.OpenDisc("game.cue")
if err != nil {
    // ...
}
defer disc.Close()

sector, err := disc.ReadSector(lba) // raw 2352-byte sector at absolute LBA
n := disc.NumTracks()
num, typ, frames, pregap, startLBA, control := disc.Track(0)
```

The reader is streaming: file handles are held open and sectors are decoded on
demand, so the whole image is never read into memory.

### Disc API

```go
func OpenDisc(path string) (*Disc, error)

func (d *Disc) ReadSector(lba int) ([]byte, error)
func (d *Disc) NumTracks() int
func (d *Disc) Track(i int) (number int, typ string, frames int, pregap int, startLBA int, control uint8)
func (d *Disc) NumTrackIndexes(i int) int
func (d *Disc) TrackIndex(i, n int) (indexNumber int, lba int)
func (d *Disc) Close() error
```

`NumTrackIndexes`/`TrackIndex` expose the per-track index map (index numbers
`>= 1`, absolute LBA); `n` is a 0-based ordinal into that list, not the index
number. Index 0 (the pregap) is not listed - it is implied for any position
below the first entry. CHD carries no index map, so its reader synthesizes a
single `INDEX 01` per track from the pregap. A cue sheet supplies the real
`INDEX 01..99`.

### cue / bin rules

- Supported track modes are raw 2352-byte only: `MODE1/2352`, `MODE2/2352`, and `AUDIO`.
- `FILE` names are resolved relative to the cue's directory. Absolute paths and
  parent-directory traversal are rejected.
- One-bin-per-track and single-bin-multiple-track layouts are both handled. An
  in-file pregap (`INDEX 00`) is read from the bin. A `PREGAP`/`POSTGAP` command
  generates silence not backed by any file.
- Sector bytes are returned verbatim (data tracks big-endian on disc, audio
  little-endian).

### Disc Errors

```go
var ErrUnsupportedDisc // path extension is neither .chd nor .cue
```


## dischash tool

`cmd/dischash` identifies disc images and prints a CRC-32 over their track data,
for matching a disc against a reference (redump-style) breakdown. It reads any
image `OpenDisc` accepts (CHD or cue/bin), recognizes Sega Saturn, Sega
Dreamcast, and PlayStation 1 discs (showing product ID and title where the
format provides them), and marks an unreadable disc as `ERROR` rather than
dropping it. A cue and its matching CHD produce identical per-track hashes.

```
go run ./cmd/dischash -file game.chd      # one disc: overall CRC-32 + ID/title
go run ./cmd/dischash -v -file game.cue   # full per-track table
go run ./cmd/dischash -dir path/to/discs  # scan a folder, discs hashed in parallel
```

`-file` and `-dir` are mutually exclusive; `-j` sets the `-dir` concurrency
(default: number of CPUs).


## Dependencies

- `github.com/bodgit/sevenzip` - 7z archive support
- `github.com/nwaples/rardecode/v2` - RAR archive support
- `github.com/klauspost/compress` - zstd decompression used by the CHD backend

ZIP and gzip archive support use Go's standard library. The cue/bin disc backend
uses only the standard library. The CHD backend's remaining codecs (zlib, LZMA,
FLAC) are stdlib or implemented in-package.


## Testing

```
go test ./...
```


## Used By

- eblitui-standalone (ROM loading for desktop UI)
- eblitui-ios (ROM loading for iOS app)

Not used by eblitui-libretro (the frontend provides ROM data directly).
