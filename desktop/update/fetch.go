// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package update

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/user-none/eblitui/desktop/netutil"
)

// fetchTimeout bounds each manifest or signature request, including
// redirects.
const fetchTimeout = 30 * time.Second

// versionPlaceholder is replaced by the offered version in the release page
// URL.
const versionPlaceholder = "{version}"

// Source is where an application's releases are published: the latest
// release's manifest and signature file, and the release pages.
type Source struct {
	manifestURL    string
	signatureURL   string
	releasePageURL string
	client         *http.Client
}

// NewSource returns the release source for the given URLs. Each must be an
// absolute HTTPS URL. In releasePageURL, {version} is replaced by the offered
// version.
func NewSource(manifestURL, signatureURL, releasePageURL string) (*Source, error) {
	for _, u := range []string{manifestURL, signatureURL, releasePageURL} {
		if err := checkHTTPSURL(u); err != nil {
			return nil, err
		}
	}
	return &Source{
		manifestURL:    manifestURL,
		signatureURL:   signatureURL,
		releasePageURL: releasePageURL,
		client:         netutil.NewClient(fetchTimeout),
	}, nil
}

// checkHTTPSURL returns an error unless s is an absolute HTTPS URL with a
// host
func checkHTTPSURL(s string) error {
	u, err := url.Parse(s)
	if err != nil {
		return fmt.Errorf("invalid update URL %q: %w", s, err)
	}
	if u.Scheme != "https" || u.Host == "" {
		return fmt.Errorf("update URL %q is not an HTTPS URL", s)
	}
	return nil
}

// FetchLatest downloads the latest release's manifest and signature file
func (s *Source) FetchLatest() (manifest, sig []byte, err error) {
	manifest, err = netutil.Download(s.client, s.manifestURL, MaxManifestSize)
	if err != nil {
		return nil, nil, err
	}
	sig, err = netutil.Download(s.client, s.signatureURL, MaxSignatureSize)
	if err != nil {
		return nil, nil, err
	}
	return manifest, sig, nil
}

// ReleasePage returns the URL of the release page for a version. The
// version must come from a verified manifest, which limits it to
// MAJOR.MINOR.PATCH.
func (s *Source) ReleasePage(version string) string {
	return strings.ReplaceAll(s.releasePageURL, versionPlaceholder, version)
}
