package netutil

import (
	"bytes"
	"fmt"
	"image"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	_ "image/jpeg" // JPEG decoder for image validation
	_ "image/png"  // PNG decoder for image validation

	_ "golang.org/x/image/webp" // WebP decoder for image validation
)

// HTTPTimeout is the default timeout for HTTP requests.
const HTTPTimeout = 10 * time.Second

// HTTPClient is a shared HTTP client with the default timeout.
var HTTPClient = &http.Client{
	Timeout: HTTPTimeout,
}

// DownloadToMemory performs a GET request and returns the response body.
func DownloadToMemory(url string) ([]byte, error) {
	resp, err := HTTPClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

// writeToFile creates parent directories as needed and writes data to savePath.
func writeToFile(savePath string, data []byte) error {
	dir := filepath.Dir(savePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return os.WriteFile(savePath, data, 0644)
}

// DownloadToFile performs a GET request and saves the response body to savePath.
// Creates parent directories as needed.
func DownloadToFile(url string, savePath string) error {
	data, err := DownloadToMemory(url)
	if err != nil {
		return err
	}

	return writeToFile(savePath, data)
}

// DownloadImgToFile performs a GET request, verifies the response body decodes
// as an image, and only then saves it to savePath. If the body is not a valid
// image no file is written, so a later scan can retry the download. Supports
// any format registered with the image package (PNG, JPEG, WebP).
func DownloadImgToFile(url string, savePath string) error {
	data, err := DownloadToMemory(url)
	if err != nil {
		return err
	}

	if _, _, err := image.Decode(bytes.NewReader(data)); err != nil {
		return fmt.Errorf("invalid image from %s: %w", url, err)
	}

	return writeToFile(savePath, data)
}
