package console

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/deeptrols/api/internal/app"
	"github.com/google/uuid"
)

// maxUploadBytes caps admin brand-asset uploads (new-api logo/favicon parity).
const maxUploadBytes = 2 << 20 // 2 MiB

// allowedUploadExts whitelists image extensions. SVG is deliberately excluded
// because SVG can embed scripts (stored-XSS via the logo URL).
var allowedUploadExts = map[string]bool{
	".png":  true,
	".jpg":  true,
	".jpeg": true,
	".webp": true,
	".gif":  true,
	".ico":  true,
}

// HandleUploadLogo stores an admin-uploaded brand image under /uploads and
// returns its public URL. Admin auth is enforced by the /api/admin middleware
// group AND rejectNonAdmin (defense in depth). Filenames are replaced with a
// random UUID so client-controlled paths can never escape the upload dir.
func HandleUploadLogo(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}
		if err := r.ParseMultipartForm(maxUploadBytes + 1<<10); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "上传失败：文件过大或格式错误"})
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "上传失败：缺少 file 字段"})
			return
		}
		defer file.Close()

		if header.Size <= 0 || header.Size > maxUploadBytes {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "上传失败：文件大小须在 1B ~ 2MiB 之间"})
			return
		}
		ext := strings.ToLower(filepath.Ext(header.Filename))
		if !allowedUploadExts[ext] {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "上传失败：仅支持 png/jpg/webp/gif/ico"})
			return
		}

		// Sniff the real content type from the first bytes; a renamed .txt
		// must not pass as an image.
		head := make([]byte, 512)
		n, _ := io.ReadFull(file, head)
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "上传失败：无法读取文件"})
			return
		}
		if !strings.HasPrefix(http.DetectContentType(head[:n]), "image/") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "上传失败：文件内容不是图片"})
			return
		}

		dir := a.Config.UploadDir
		if dir == "" {
			dir = "./uploads"
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "上传失败：无法创建存储目录"})
			return
		}
		name := uuid.NewString() + ext
		dst, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "上传失败：无法写入文件"})
			return
		}
		defer dst.Close()
		if _, err := io.Copy(dst, file); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "上传失败：写入中断"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"url":  "/uploads/" + name,
			"name": name,
		})
	}
}
