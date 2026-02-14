package api

import (
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/messenger/server/internal/media"
)

type mediaHandler struct {
	media *media.Service
}

func (h *mediaHandler) upload(w http.ResponseWriter, r *http.Request) {
	contentLength := r.ContentLength
	if contentLength <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "Content-Length required")
		return
	}
	if contentLength > 100*1024*1024 {
		writeError(w, http.StatusRequestEntityTooLarge, "file_too_large", "Max file size 100MB")
		return
	}

	contentURI, err := h.media.Upload(r.Context(), r.Body, contentLength)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "upload_failed", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"content_uri": contentURI,
	})
}

func (h *mediaHandler) download(w http.ResponseWriter, r *http.Request) {
	// Path: /api/v1/media/blob:sha256:hex — wildcard * captures rest (может прийти как blob%3Asha256%3Ahex)
	contentURI := strings.TrimPrefix(chi.URLParam(r, "*"), "/")
	if contentURI == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "content_uri required")
		return
	}
	if decoded, err := url.PathUnescape(contentURI); err == nil {
		contentURI = decoded
	}
	if !strings.HasPrefix(contentURI, "blob:sha256:") {
		contentURI = "blob:sha256:" + contentURI
	}

	rc, err := h.media.Download(r.Context(), contentURI)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "File not found")
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment")
	_, _ = io.Copy(w, rc)
}
