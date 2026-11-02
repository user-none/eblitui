// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package netutil

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

// HTTPTimeout is the default timeout for HTTP requests.
const HTTPTimeout = 10 * time.Second

// Download performs a GET request with client and returns the response
// body. A status other than 200 or a body larger than limit bytes returns an
// error.
func Download(client *http.Client, url string, limit int64) ([]byte, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("download from %s exceeds %d bytes", url, limit)
	}
	return data, nil
}
