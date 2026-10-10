package assistant

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	maxBody       = 8 << 10
	DefaultPerMin = 10
)

// Handler serves POST /ask for a Service, limiting each client IP to perMin questions a minute
// (DefaultPerMin if perMin <= 0).
func Handler(s *Service, perMin int) http.Handler {
	if perMin <= 0 {
		perMin = DefaultPerMin
	}
	limit := NewLimiter(perMin, time.Minute)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ok, wait := limit.Allow(ClientIP(r)); !ok {
			w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
			writeError(w, http.StatusTooManyRequests, "too many questions, try again in a minute")
			return
		}
		var req Request
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
		if err != nil {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		if err := json.Unmarshal(body, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
			return
		}
		ans, err := s.Ask(r.Context(), req)
		switch {
		case errors.Is(err, ErrInvalid):
			writeError(w, http.StatusBadRequest, strings.TrimPrefix(err.Error(), ErrInvalid.Error()+": "))
		case err != nil:
			writeError(w, http.StatusBadGateway, "the assistant is not reachable right now, try again shortly")
		default:
			writeJSON(w, http.StatusOK, ans)
		}
	})
}

// NotConfigured answers 503: no Bedrock model is set.
func NotConfigured() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusServiceUnavailable, "the assistant is not configured (set AAHAT_ANTHROPIC_API_KEY or AAHAT_BEDROCK_MODEL)")
	})
}

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

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// ClientIP is the caller's address. Behind the local reverse proxy (Caddy on the same host) it
// is the last X-Forwarded-For entry, the one the proxy itself appended; a direct caller cannot
// choose its bucket by sending the header.
func ClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			if last := strings.TrimSpace(parts[len(parts)-1]); last != "" {
				return last
			}
		}
	}
	return host
}

// Limiter allows each key a fixed number of events per sliding window. In memory: one process.
type Limiter struct {
	max    int
	window time.Duration
	now    func() time.Time

	mu   sync.Mutex
	seen map[string][]time.Time
}

func NewLimiter(max int, window time.Duration) *Limiter {
	return &Limiter{max: max, window: window, now: time.Now, seen: map[string][]time.Time{}}
}

// Allow records an event for key, or reports how long until the next one is allowed.
func (l *Limiter) Allow(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	cutoff := now.Add(-l.window)
	if len(l.seen) > 10000 { // forget idle clients before the map grows without bound
		for k, times := range l.seen {
			if len(times) == 0 || !times[len(times)-1].After(cutoff) {
				delete(l.seen, k)
			}
		}
	}
	times := l.seen[key]
	for len(times) > 0 && !times[0].After(cutoff) {
		times = times[1:]
	}
	if len(times) >= l.max {
		l.seen[key] = times
		return false, times[0].Add(l.window).Sub(now)
	}
	l.seen[key] = append(times, now)
	return true, 0
}
