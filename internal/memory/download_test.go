package memory

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDownloadToFileRejectsContentLength(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "5")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello"))
	}))
	defer server.Close()

	tmp, err := os.CreateTemp(t.TempDir(), "download-*")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	defer tmp.Close()

	url := server.URL + "/model.onnx?token=secret"
	err = downloadToFile(context.Background(), server.Client(), url, tmp, 4)
	if err == nil {
		t.Fatal("expected error for oversized content-length")
	}
	assertNoURLLeak(t, err, url)
}

func TestDownloadToFileRejectsStreamOverflow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		for i := 0; i < 6; i++ {
			_, _ = w.Write([]byte("a"))
		}
	}))
	defer server.Close()

	tmp, err := os.CreateTemp(t.TempDir(), "download-*")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	defer tmp.Close()

	url := server.URL + "/model.onnx?token=secret"
	err = downloadToFile(context.Background(), server.Client(), url, tmp, 5)
	if err == nil {
		t.Fatal("expected error for oversized stream")
	}
	assertNoURLLeak(t, err, url)
}

func TestDownloadToFileRejectsNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	tmp, err := os.CreateTemp(t.TempDir(), "download-*")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	defer tmp.Close()

	url := server.URL + "/model.onnx?token=secret"
	err = downloadToFile(context.Background(), server.Client(), url, tmp, 10)
	if err == nil {
		t.Fatal("expected error for non-2xx response")
	}
	assertNoURLLeak(t, err, url)
}

func TestEnsureFileWithSHADetectsMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("abc"))
	}))
	defer server.Close()

	url := server.URL + "/tokenizer.json?token=secret"
	path := filepath.Join(t.TempDir(), "tokenizer.json")
	err := ensureFileWithSHA(context.Background(), server.Client(), path, url, "deadbeef", 1024)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected checksum mismatch, got %v", err)
	}
	assertNoURLLeak(t, err, url)
}

func assertNoURLLeak(t *testing.T, err error, rawURL string) {
	t.Helper()
	if err == nil {
		return
	}
	msg := err.Error()
	if strings.Contains(msg, rawURL) {
		t.Fatalf("error leaked full URL: %s", msg)
	}
	if strings.Contains(msg, "token=secret") {
		t.Fatalf("error leaked query string: %s", msg)
	}
}
