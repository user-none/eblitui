package standalone

import (
	_ "embed"
)

//go:embed assets/placeholder.png
var placeholderImageData []byte

//go:embed assets/missing.png
var missingArtImageData []byte

// GetPlaceholderImageData returns the raw embedded placeholder image data
// used as the loading indicator while artwork is being loaded asynchronously.
func (a *App) GetPlaceholderImageData() []byte {
	return placeholderImageData
}

// GetMissingArtImageData returns the raw embedded missing-art image data
// shown for games that have no artwork file.
func (a *App) GetMissingArtImageData() []byte {
	return missingArtImageData
}
