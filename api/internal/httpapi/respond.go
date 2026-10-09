package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/ayush00git/aahat/api/internal/alert"
	"github.com/ayush00git/aahat/api/internal/data"
	"github.com/ayush00git/aahat/api/internal/store"
)

const maxBody = 64 << 10

func writeJSON(w http.ResponseWriter, status int, v any) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		http.Error(w, `{"error":"encoding response"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	w.Write(b.Bytes())
}

// writeRaw passes a published file through unchanged.
func writeRaw(w http.ResponseWriter, contentType string, body []byte) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=60")
	w.WriteHeader(http.StatusOK)
	w.Write(body)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// fail maps an error to a status code. notFound is the message for a missing
// lake, file, subscription or event.
func (s *server) fail(w http.ResponseWriter, r *http.Request, err error, notFound string) {
	switch {
	case errors.Is(err, data.ErrNotFound), errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, notFound)
	case errors.Is(err, alert.ErrInvalid):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		s.log.ErrorContext(r.Context(), "request failed", "method", r.Method, "path", r.URL.Path, "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

// readJSON decodes a size-limited JSON body into v.
func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		return false
	}
	if err := json.Unmarshal(body, v); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON body: %v", err))
		return false
	}
	return true
}
