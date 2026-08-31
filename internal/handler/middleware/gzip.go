package middleware

import (
	"compress/gzip"
	"net/http"
	"strings"
)

// Gzip compresses response bodies for clients that advertise
// Accept-Encoding: gzip. It skips /uploads (binary assets)
// and preserves SSE flushing so streaming responses stay live.
func Gzip(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/uploads") ||
			!strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Add("Vary", "Accept-Encoding")
		gz := gzip.NewWriter(w)
		defer gz.Close()
		next.ServeHTTP(&gzipResponseWriter{ResponseWriter: w, Writer: gz}, r)
	})
}

type gzipResponseWriter struct {
	http.ResponseWriter
	Writer *gzip.Writer
	header bool
}

func (g *gzipResponseWriter) WriteHeader(code int) {
	if !g.header {
		g.Header().Set("Content-Encoding", "gzip")
		g.header = true
	}
	g.ResponseWriter.WriteHeader(code)
}

func (g *gzipResponseWriter) Write(b []byte) (int, error) {
	if !g.header {
		g.WriteHeader(http.StatusOK)
	}
	return g.Writer.Write(b)
}

// Flush keeps SSE/streaming responses flowing through the gzip writer.
func (g *gzipResponseWriter) Flush() {
	_ = g.Writer.Flush()
	if f, ok := g.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap supports http.ResponseController (Go 1.20+) for handlers that need
// the underlying writer (e.g. SSE hijacking is not used, but trailers are).
func (g *gzipResponseWriter) Unwrap() http.ResponseWriter {
	return g.ResponseWriter
}

var _ http.Flusher = (*gzipResponseWriter)(nil)
