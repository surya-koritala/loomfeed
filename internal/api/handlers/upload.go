package handlers

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/RoamXAI/loomfeed/internal/api"
	"github.com/RoamXAI/loomfeed/internal/api/middleware"
	"github.com/RoamXAI/loomfeed/internal/config"
	"github.com/RoamXAI/loomfeed/internal/moderation"
)

// UploadHandler handles file upload endpoints.
type UploadHandler struct {
	uploadsDir string
	cfg        config.UploadsConfig
	moderator  moderation.ImageModerator
}

// NewUploadHandler creates a new UploadHandler. cfg controls whether
// uploads are accepted at all and whether each upload is screened by
// Content Safety before being written to disk. moderator may be nil
// when cfg.ContentSafety.Enabled is false.
func NewUploadHandler(uploadsDir string, cfg config.UploadsConfig, moderator moderation.ImageModerator) *UploadHandler {
	return &UploadHandler{uploadsDir: uploadsDir, cfg: cfg, moderator: moderator}
}

var allowedMimeTypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/gif":  ".gif",
	"image/webp": ".webp",
}

// imageMagicBytes maps MIME types to their file signature (magic bytes).
// Used to validate that uploaded files are genuine images, not just renamed.
var imageMagicBytes = map[string][]byte{
	"image/jpeg": {0xFF, 0xD8, 0xFF},
	"image/png":  {0x89, 0x50, 0x4E, 0x47},
	"image/gif":  {0x47, 0x49, 0x46},
	"image/webp": {0x52, 0x49, 0x46, 0x46}, // RIFF header; WebP also has "WEBP" at offset 8
}

// isValidImageMagic checks whether buf starts with a recognized image magic byte sequence.
func isValidImageMagic(buf []byte) bool {
	for _, magic := range imageMagicBytes {
		if len(buf) >= len(magic) && bytes.Equal(buf[:len(magic)], magic) {
			return true
		}
	}
	return false
}

// detectImageType returns the MIME type based on magic bytes, or "" if unrecognized.
func detectImageType(buf []byte) string {
	for mime, magic := range imageMagicBytes {
		if len(buf) >= len(magic) && bytes.Equal(buf[:len(magic)], magic) {
			return mime
		}
	}
	return ""
}

// Upload handles POST /api/v1/upload.
// Accepts a multipart form with a "file" field (max 5 MB).
//
// Pipeline: auth -> uploads-enabled gate -> size limit -> magic-byte
// validation -> Content Safety moderation -> disk write. Content Safety
// runs on the in-memory bytes before the file ever touches disk; a
// blocked image is never persisted. Fails closed: if the moderator
// returns an error (network hiccup, rate limit, bad credentials), the
// upload is rejected rather than silently allowed through.
func (h *UploadHandler) Upload(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	if !h.cfg.Enabled {
		api.Error(w, http.StatusServiceUnavailable, "image uploads are temporarily disabled")
		return
	}

	// Parse multipart form (max 5 MB)
	if err := r.ParseMultipartForm(5 << 20); err != nil {
		api.Error(w, http.StatusBadRequest, "request too large or not multipart")
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		api.Error(w, http.StatusBadRequest, "file field is required")
		return
	}
	defer func() { _ = file.Close() }()

	// Read the full file into memory. 5 MB cap is enforced by
	// ParseMultipartForm above, so this is bounded.
	fullBytes, err := io.ReadAll(file)
	if err != nil {
		api.Error(w, http.StatusBadRequest, "failed to read upload")
		return
	}

	// Validate using magic bytes — rejects files that don't start with a
	// recognized image signature, regardless of extension or Content-Type header
	if !isValidImageMagic(fullBytes) {
		api.Error(w, http.StatusBadRequest, "only jpg, png, gif, and webp images are allowed")
		return
	}

	// Determine the actual type from magic bytes (authoritative)
	contentType := detectImageType(fullBytes)

	allowedExt, ok := allowedMimeTypes[contentType]
	if !ok {
		api.Error(w, http.StatusBadRequest, "only jpg, png, gif, and webp images are allowed")
		return
	}

	// Content Safety moderation. Runs before we persist anything so a
	// blocked image is never written to disk or served publicly. If the
	// operator has enabled uploads without Content Safety, we log a
	// prominent warning on every upload so the misconfiguration is
	// visible — we still allow the upload through in that mode, because
	// blocking silently would break the feature without any signal.
	if h.cfg.ContentSafety.Enabled {
		if h.moderator == nil {
			log.Printf("upload: CONTENT_SAFETY_ENABLED=true but moderator is not configured, failing closed")
			api.Error(w, http.StatusServiceUnavailable, "moderation unavailable")
			return
		}
		decision, merr := h.moderator.Check(r.Context(), fullBytes)
		if merr != nil {
			log.Printf("upload: content-safety error for participant %s: %v", claims.ParticipantID, merr)
			api.Error(w, http.StatusServiceUnavailable, "moderation check failed, try again")
			return
		}
		if !decision.Allowed {
			log.Printf("upload: blocked image from participant %s: category=%s severity=%d",
				claims.ParticipantID, decision.Category, decision.Severity)
			api.Error(w, http.StatusBadRequest, "image failed safety check")
			return
		}
	} else {
		log.Printf("upload: WARNING — CONTENT_SAFETY_ENABLED=false, upload from participant %s not moderated",
			claims.ParticipantID)
	}

	// Generate unique filename
	id := uuid.New().String()
	timestamp := time.Now().Unix()
	filename := fmt.Sprintf("%d_%s%s", timestamp, id, allowedExt)
	destPath := filepath.Join(h.uploadsDir, filename)

	// Ensure uploads dir exists
	if err := os.MkdirAll(h.uploadsDir, 0755); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to create uploads directory")
		return
	}

	if err := os.WriteFile(destPath, fullBytes, 0644); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to save file")
		return
	}

	url := "/uploads/" + filename
	api.JSON(w, http.StatusOK, map[string]string{"url": url, "filename": filename})
}
