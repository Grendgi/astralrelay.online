package api

import (
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/messenger/server/internal/media"
)

type mediaHandler struct {
	media *media.Service
}

// parseRange parses "bytes=N-" or "bytes=N-M", returns (offset, length). length 0 = to end.
func parseRange(s string) (offset, length int64, ok bool) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(strings.ToLower(s), "bytes=") {
		return 0, 0, false
	}
	s = s[6:]
	parts := strings.SplitN(s, "-", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	var n, m int64
	if parts[0] != "" {
		if _, err := strconv.ParseInt(parts[0], 10, 64); err != nil {
			return 0, 0, false
		}
		n, _ = strconv.ParseInt(parts[0], 10, 64)
	}
	if parts[1] != "" {
		if _, err := strconv.ParseInt(parts[1], 10, 64); err != nil {
			return 0, 0, false
		}
		m, _ = strconv.ParseInt(parts[1], 10, 64)
		length = m - n + 1
	}
	offset = n
	return offset, length, true
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

	offset, length, hasRange := parseRange(r.Header.Get("Range"))

	rc, info, err := h.media.Download(r.Context(), contentURI, offset, length)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "File not found")
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment")
	w.Header().Set("Accept-Ranges", "bytes")
	if hasRange && info != nil && info.IsRange && info.TotalSize > 0 {
		end := offset + length - 1
		if length <= 0 {
			end = info.TotalSize - 1
		}
		w.Header().Set("Content-Range",
			"bytes "+strconv.FormatInt(offset, 10)+"-"+
				strconv.FormatInt(end, 10)+"/"+
				strconv.FormatInt(info.TotalSize, 10))
		w.WriteHeader(http.StatusPartialContent)
	}
	_, _ = io.Copy(w, rc)
}
