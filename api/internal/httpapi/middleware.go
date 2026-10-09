package httpapi

import (
	"log/slog"
	"net/http"
	"slices"
	"strings"
)

// cors allows any origin: the map, the villager app and the dashboard are
// served from other domains, and nothing here relies on cookies.
func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Access-Control-Allow-Origin", "*")
		h.Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		h.Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Aahat-Signature")
		h.Set("Access-Control-Max-Age", "86400")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func recoverer(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				log.ErrorContext(r.Context(), "panic", "path", r.URL.Path, "panic", v)
				writeError(w, http.StatusInternalServerError, "internal error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// fallback answers requests no route matched with JSON: 405 if the path
// exists under another method, else 404. (ServeMux's own replies are plain
// text, and a catch-all "/" route stops it from sending 405 at all.)
func fallback(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var allowed []string
		for _, m := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
			probe := r.Clone(r.Context())
			probe.Method = m
			if _, pattern := mux.Handler(probe); pattern != "/" && pattern != "" {
				allowed = append(allowed, m)
			}
		}
		if len(allowed) > 0 && !slices.Contains(allowed, r.Method) {
			w.Header().Set("Allow", strings.Join(allowed, ", "))
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		writeError(w, http.StatusNotFound, "not found")
	})
}
