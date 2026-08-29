package console

import (
	"bytes"
	"encoding/base64"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/config"
	"github.com/google/uuid"
)

// tinyPNG is a 1x1 transparent PNG (valid image bytes for content sniffing).
var tinyPNG = func() []byte {
	b, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==")
	return b
}()

func uploadApp(t *testing.T) *app.App {
	t.Helper()
	return &app.App{Config: &config.Config{UploadDir: t.TempDir()}}
}

func uploadRequest(t *testing.T, a *app.App, filename string, content []byte, admin bool) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := fw.Write(content); err != nil {
		t.Fatalf("write content: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/admin/settings/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if admin {
		req = setAdminContext(req, uuid.New().String())
	} else {
		req = setUserContext(req, uuid.New().String())
	}
	w := httptest.NewRecorder()
	HandleUploadLogo(a).ServeHTTP(w, req)
	return w
}

func TestHandleUploadLogo_SuccessStoresFile(t *testing.T) {
	a := uploadApp(t)
	w := uploadRequest(t, a, "logo.png", tinyPNG, true)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"url":"/uploads/`) {
		t.Fatalf("response missing url: %s", w.Body.String())
	}
	name := ""
	for _, p := range strings.Split(w.Body.String(), `"`) {
		if strings.HasPrefix(p, "/uploads/") {
			name = strings.TrimPrefix(p, "/uploads/")
			break
		}
	}
	if name == "" || strings.Contains(name, "..") {
		t.Fatalf("unexpected stored name from body: %s", w.Body.String())
	}
	stored, err := os.ReadFile(filepath.Join(a.Config.UploadDir, name))
	if err != nil {
		t.Fatalf("stored file missing: %v", err)
	}
	if !bytes.Equal(stored, tinyPNG) {
		t.Fatal("stored content mismatch")
	}
}

func TestHandleUploadLogo_PathTraversalFilenameIsSanitized(t *testing.T) {
	a := uploadApp(t)
	w := uploadRequest(t, a, "../../evil.png", tinyPNG, true)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if _, err := os.Stat(filepath.Join(a.Config.UploadDir, "evil.png")); !os.IsNotExist(err) {
		t.Fatalf("client filename must not be used verbatim (evil.png exists? err=%v)", err)
	}
	if strings.Contains(w.Body.String(), "evil") || strings.Contains(w.Body.String(), "..") {
		t.Fatalf("response leaked client path: %s", w.Body.String())
	}
}

func TestHandleUploadLogo_RejectsNonImageContent(t *testing.T) {
	a := uploadApp(t)
	// Renamed .png carrying text: content sniffing must reject it.
	w := uploadRequest(t, a, "logo.png", []byte("not an image at all"), true)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", w.Code, w.Body.String())
	}
	// Unsupported extension.
	w = uploadRequest(t, a, "logo.txt", tinyPNG, true)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("txt status = %d, want 400", w.Code)
	}
	// SVG is disallowed (stored-XSS vector).
	w = uploadRequest(t, a, "logo.svg", []byte("<svg xmlns=\"http://www.w3.org/2000/svg\"/>"), true)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("svg status = %d, want 400", w.Code)
	}
}

func TestHandleUploadLogo_RejectsOversizedFile(t *testing.T) {
	a := uploadApp(t)
	w := uploadRequest(t, a, "big.png", bytes.Repeat([]byte{0x61}, maxUploadBytes+1), true)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", w.Code, w.Body.String())
	}
}

func TestHandleUploadLogo_RequiresAdmin(t *testing.T) {
	a := uploadApp(t)
	// Non-admin (role=user) context.
	w := uploadRequest(t, a, "logo.png", tinyPNG, false)
	if w.Code != http.StatusForbidden {
		t.Fatalf("user status = %d, want 403", w.Code)
	}
	// No context at all.
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "logo.png")
	_, _ = fw.Write(tinyPNG)
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/settings/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w2 := httptest.NewRecorder()
	HandleUploadLogo(a).ServeHTTP(w2, req)
	if w2.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous status = %d, want 401", w2.Code)
	}
}
