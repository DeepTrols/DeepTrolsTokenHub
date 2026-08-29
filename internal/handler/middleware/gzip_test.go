package middleware

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGzip_CompressesWhenAccepted(t *testing.T) {
	handler := Gzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/console/me", nil)
	req.Header.Set("Accept-Encoding", "gzip, deflate")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", w.Header().Get("Content-Encoding"))
	}
	if !strings.Contains(w.Header().Get("Vary"), "Accept-Encoding") {
		t.Fatal("Vary: Accept-Encoding missing")
	}
	zr, err := gzip.NewReader(w.Body)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	body, _ := io.ReadAll(zr)
	if string(body) != `{"ok":true}` {
		t.Fatalf("body = %q", body)
	}
}

func TestGzip_PassthroughWhenNotAccepted(t *testing.T) {
	handler := Gzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("plain"))
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/console/me", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Header().Get("Content-Encoding") != "" {
		t.Fatalf("unexpected Content-Encoding = %q", w.Header().Get("Content-Encoding"))
	}
	if w.Body.String() != "plain" {
		t.Fatalf("body = %q", w.Body.String())
	}
}

func TestGzip_SkipsUploads(t *testing.T) {
	handler := Gzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("png-bytes"))
	}))
	req := httptest.NewRequest(http.MethodGet, "/uploads/logo.png", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Header().Get("Content-Encoding") != "" {
		t.Fatalf("uploads must not be gzipped, got %q", w.Header().Get("Content-Encoding"))
	}
	if w.Body.String() != "png-bytes" {
		t.Fatalf("body = %q", w.Body.String())
	}
}
