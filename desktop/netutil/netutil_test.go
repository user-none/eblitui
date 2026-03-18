package netutil

import (
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
