// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package signing

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"
)

// newKey returns a private key and its public key in algorithm:base64 form
func newKey(t *testing.T) (ed25519.PrivateKey, string) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return priv, "ed25519:" + base64.StdEncoding.EncodeToString(pub)
}

func sigLine(priv ed25519.PrivateKey, data []byte) string {
	return "ed25519 " + base64.StdEncoding.EncodeToString(ed25519.Sign(priv, data)) + "\n"
}

func TestParseKeys(t *testing.T) {
	_, good := newKey(t)
	_, good2 := newKey(t)

	keys, errs := ParseKeys([]string{good, good2})
	if len(errs) != 0 || len(keys.ed25519) != 2 {
		t.Errorf("two good keys: %d keys, errors %v", len(keys.ed25519), errs)
	}

	short := "ed25519:" + base64.StdEncoding.EncodeToString(make([]byte, 31))
	bad := []string{"", "ed25519", "ed25519:", "ed25519:not base64!", short, "mldsa:AAAA"}
	keys, errs = ParseKeys(append([]string{good}, bad...))
	if len(keys.ed25519) != 1 {
		t.Errorf("good key among bad ones: got %d keys, want 1", len(keys.ed25519))
	}
	if len(errs) != len(bad) {
		t.Errorf("got %d errors, want %d: %v", len(errs), len(bad), errs)
	}

	keys, _ = ParseKeys(bad)
	if !keys.Empty() {
		t.Error("only bad keys should leave no usable keys")
	}
	keys, _ = ParseKeys(nil)
	if !keys.Empty() {
		t.Error("no keys should be empty")
	}
}

func TestVerify(t *testing.T) {
	priv, pub := newKey(t)
	otherPriv, otherPub := newKey(t)
	keys, _ := ParseKeys([]string{pub})
	data := []byte(`{"version":1}`)

	tests := []struct {
		name string
		data []byte
		sig  string
		want bool
	}{
		{"signed", data, sigLine(priv, data), true},
		{"second line signed", data, sigLine(otherPriv, data) + sigLine(priv, data), true},
		{"unknown algorithm skipped", data, "mldsa AAAA\n" + sigLine(priv, data), true},
		{"untrusted key", data, sigLine(otherPriv, data), false},
		{"other data", []byte(`{"version":2}`), sigLine(priv, data), false},
		{"empty", data, "", false},
		{"malformed", data, "ed25519 not-base64\n", false},
		{"too many lines", data, strings.Repeat("x\n", MaxSignatureLines) + sigLine(priv, data), false},
		{"too large", data, sigLine(priv, data) + strings.Repeat(" ", MaxSignatureSize), false},
	}
	for _, tt := range tests {
		if got := Verify(tt.data, []byte(tt.sig), keys); got != tt.want {
			t.Errorf("%s: Verify = %v, want %v", tt.name, got, tt.want)
		}
	}

	both, _ := ParseKeys([]string{pub, otherPub})
	if !Verify(data, []byte(sigLine(otherPriv, data)), both) {
		t.Error("signature by the second trusted key rejected")
	}
	if Verify(data, []byte(sigLine(priv, data)), Keys{}) {
		t.Error("signature verified with no keys")
	}
}
