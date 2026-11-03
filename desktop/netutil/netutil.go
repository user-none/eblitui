// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package netutil

import (
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"time"
)

// HTTPTimeout is the default timeout for HTTP requests.
const HTTPTimeout = 10 * time.Second

// userAgent is the User-Agent header Download sends; see SetUserAgent
var userAgent atomic.Value

// SetUserAgent sets the User-Agent header Download sends, such as
// "erings/0.4.6". Empty sends Go's default.
func SetUserAgent(ua string) {
	userAgent.Store(ua)
}

// Download performs a GET request with client and returns the response
// body, sending the User-Agent set by SetUserAgent. A status other than 200
// or a body larger than limit bytes returns an error.
func Download(client *http.Client, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if ua, _ := userAgent.Load().(string); ua != "" {
		req.Header.Set("User-Agent", ua)
	}
	resp, err := client.Do(req)
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
