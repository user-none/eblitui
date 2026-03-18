package netutil

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
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

// DownloadToFile performs a GET request and saves the response body to savePath.
// Creates parent directories as needed.
func DownloadToFile(url string, savePath string) error {
	data, err := DownloadToMemory(url)
	if err != nil {
		return err
	}

	dir := filepath.Dir(savePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return os.WriteFile(savePath, data, 0644)
}
