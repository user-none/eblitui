// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package netutil

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"time"
)

// HTTPTimeout is the default timeout for HTTP requests.
const HTTPTimeout = 10 * time.Second

// maxRedirects is the most redirects a client from NewClient follows for
// one request.
const maxRedirects = 10

// ErrNotHTTPS is returned for a request or redirect to a URL that is not
// HTTPS.
var ErrNotHTTPS = errors.New("not an HTTPS URL")

// NewClient returns an HTTP client that only makes HTTPS requests. A request
// for any other scheme fails with ErrNotHTTPS before anything is sent, which
// also covers redirects; at most maxRedirects redirects are followed.
// Requests go through http.DefaultTransport, which verifies server
// certificates against the system's trusted roots.
func NewClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: httpsOnly{},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > maxRedirects {
				return fmt.Errorf("more than %d redirects", maxRedirects)
			}
			return nil
		},
	}
}

// httpsOnly is a transport that refuses non-HTTPS requests and sends the
// rest through http.DefaultTransport, looked up per request.
type httpsOnly struct{}

func (httpsOnly) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != "https" {
		// A RoundTripper must close the request body, even on error.
		if req.Body != nil {
			req.Body.Close()
		}
		return nil, ErrNotHTTPS
	}
	return http.DefaultTransport.RoundTrip(req)
}

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
