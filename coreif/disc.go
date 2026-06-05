package coreif

// DiscReader is a streaming reader over a CD/disc image. Every signature
// uses only stdlib types and no named aggregate, so a concrete reader
// satisfies this interface structurally without importing this package.
type DiscReader interface {
	// ReadSector returns the raw 2352-byte sector at the given LBA.
	ReadSector(lba int) ([]byte, error)

	// NumTracks returns the number of tracks on the disc.
	NumTracks() int

	// Track returns the TOC fields for track index i in [0, NumTracks).
	Track(i int) (number int, typ string, frames int, pregap int, startLBA int, control uint8)

	// NumTrackIndexes returns the count of index entries (index numbers >= 1)
	// for track index i in [0, NumTracks). Index 0 (the pregap) is not reported
	// here; it is implied for any FAD below the first entry.
	NumTrackIndexes(i int) int

	// TrackIndex returns the nth index entry of track index i. n is a 0-based
	// ordinal into the exposed list in [0, NumTrackIndexes(i)), not the index
	// number; entry 0 is the lowest-numbered exposed index (normally INDEX 01).
	// The returned lba is the absolute disc LBA of the index.
	TrackIndex(i, n int) (indexNumber int, lba int)

	// Close releases the underlying resources.
	Close() error
}

// DiscInfo holds the disc-derived facts the UI needs to group a game's
// discs and resolve metadata. It contains only values read off the disc
// itself; it carries no knowledge of any external catalog (e.g. RDB)
// serial conventions.
type DiscInfo struct {
	// ProductNumber is the disc's product number. It is the same for
	// every disc of a multi-disc game, so the UI also uses it as the
	// library/grouping key.
	ProductNumber string

	// DiscNumber is the 1-based position of this disc within the game.
	DiscNumber int

	// DiscTotal is the total number of discs the game spans.
	DiscTotal int

	// Title is the on-disc game title, used as a display-name fallback.
	Title string
}

// DiscIdentifier is an optional interface a CoreFactory may implement so
// the UI can derive a disc's identifying information without instantiating
// an emulator. Used to group discs and resolve metadata for disc-based
// systems.
type DiscIdentifier interface {
	// DiscInfo returns the disc's derived information and true when it
	// can be read.
	DiscInfo(disc DiscReader) (info DiscInfo, ok bool)
}
