# eblitui-romloader

A shared ROM loading utility for eblitui UIs. Handles loading ROM files
from raw files and compressed archives (ZIP, 7z, gzip/tar.gz, RAR).

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


## dischash tool

`cmd/dischash` identifies disc images and prints a CRC-32 over their track data,
for matching CHDs against a reference (redump-style) breakdown. It reads discs
through the CHD reader, recognizes Sega Saturn, Sega Dreamcast, and PlayStation 1
discs (showing product ID and title where the format provides them), and marks an
unreadable disc as `ERROR` rather than dropping it.

```
go run ./cmd/dischash -file game.chd      # one disc: overall CRC-32 + ID/title
go run ./cmd/dischash -v -file game.chd   # full per-track table
go run ./cmd/dischash -dir path/to/chds   # scan a folder, discs hashed in parallel
```

`-file` and `-dir` are mutually exclusive; `-j` sets the `-dir` concurrency
(default: number of CPUs).


## Dependencies

- `github.com/bodgit/sevenzip` - 7z archive support
- `github.com/nwaples/rardecode/v2` - RAR archive support

ZIP and gzip support use Go's standard library.


## Testing

```
go test ./...
```


## Used By

- eblitui-standalone (ROM loading for desktop UI)
- eblitui-ios (ROM loading for iOS app)

Not used by eblitui-libretro (the frontend provides ROM data directly).
