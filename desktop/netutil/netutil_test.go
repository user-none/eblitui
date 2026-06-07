package netutil

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDownloadToMemory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello"))
	}))
	defer server.Close()

	origClient := HTTPClient
	HTTPClient = server.Client()
	defer func() { HTTPClient = origClient }()

	data, err := DownloadToMemory(server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("got %q, want %q", string(data), "hello")
	}
}

func TestDownloadToMemory404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	origClient := HTTPClient
	HTTPClient = server.Client()
	defer func() { HTTPClient = origClient }()

	_, err := DownloadToMemory(server.URL)
	if err == nil {
		t.Error("expected error for 404")
	}
}

func TestDownloadToFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("file content"))
	}))
	defer server.Close()

	origClient := HTTPClient
	HTTPClient = server.Client()
	defer func() { HTTPClient = origClient }()

	tmpDir := t.TempDir()
	savePath := filepath.Join(tmpDir, "sub", "test.dat")

	err := DownloadToFile(server.URL, savePath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(savePath)
	if err != nil {
		t.Fatalf("failed to read saved file: %v", err)
	}
	if string(data) != "file content" {
		t.Errorf("got %q, want %q", string(data), "file content")
	}
}

func encodePNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("failed to encode png: %v", err)
	}
	return buf.Bytes()
}

func encodeJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("failed to encode jpeg: %v", err)
	}
	return buf.Bytes()
}

func serveBytes(body []byte) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(body)
	}))
}

func TestDownloadImgToFilePNG(t *testing.T) {
	want := encodePNG(t)
	server := serveBytes(want)
	defer server.Close()

	origClient := HTTPClient
	HTTPClient = server.Client()
	defer func() { HTTPClient = origClient }()

	savePath := filepath.Join(t.TempDir(), "sub", "boxart.png")

	if err := DownloadImgToFile(server.URL, savePath); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := os.ReadFile(savePath)
	if err != nil {
		t.Fatalf("failed to read saved file: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("saved bytes do not match served image")
	}
}

func TestDownloadImgToFileJPEG(t *testing.T) {
	want := encodeJPEG(t)
	server := serveBytes(want)
	defer server.Close()

	origClient := HTTPClient
	HTTPClient = server.Client()
	defer func() { HTTPClient = origClient }()

	savePath := filepath.Join(t.TempDir(), "boxart.png")

	if err := DownloadImgToFile(server.URL, savePath); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := os.ReadFile(savePath)
	if err != nil {
		t.Fatalf("failed to read saved file: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("saved bytes do not match served image")
	}
}

func TestDownloadImgToFileInvalid(t *testing.T) {
	server := serveBytes([]byte("not an image"))
	defer server.Close()

	origClient := HTTPClient
	HTTPClient = server.Client()
	defer func() { HTTPClient = origClient }()

	savePath := filepath.Join(t.TempDir(), "boxart.png")

	if err := DownloadImgToFile(server.URL, savePath); err == nil {
		t.Error("expected error for invalid image body")
	}

	if _, err := os.Stat(savePath); !os.IsNotExist(err) {
		t.Errorf("expected no file written, stat err = %v", err)
	}
}
