package achievements

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

// zeroReader returns zero bytes without end.
type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

// TestReadResponseBody tests that a body up to the limit is read whole and
// a longer one is refused.
func TestReadResponseBody(t *testing.T) {
	body, err := readResponseBody(bytes.NewReader([]byte(`{"Success":true}`)))
	if err != nil || string(body) != `{"Success":true}` {
		t.Fatalf("small body = %q, %v", body, err)
	}

	body, err = readResponseBody(io.LimitReader(zeroReader{}, maxResponseBytes))
	if err != nil || len(body) != maxResponseBytes {
		t.Fatalf("body at the limit: %d bytes, %v", len(body), err)
	}

	if _, err := readResponseBody(io.LimitReader(zeroReader{}, maxResponseBytes+1)); !errors.Is(err, errResponseTooLarge) {
		t.Fatalf("body over the limit: expected errResponseTooLarge, got %v", err)
	}

	if _, err := readResponseBody(zeroReader{}); !errors.Is(err, errResponseTooLarge) {
		t.Fatalf("endless body: expected errResponseTooLarge, got %v", err)
	}
}
