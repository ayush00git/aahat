package research

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	maxBody        = 8 << 10
	maxNameChars   = 120
	firstYear      = 2016 // Sentinel-2's first post-monsoon season
	maxYears       = 10
	defaultYears   = 6 // the current season and the five before it
	resolveTimeout = 20 * time.Second
	summaryTimeout = 20 * time.Second
)

// Service is the researcher API. Resolver and Summarizer are optional: without a resolver a
// request must carry coordinates, without a summarizer finished jobs get no summary.
type Service struct {
	Store      Store
	Resolver   Resolver
	Summarizer Summarizer
	Now        func() time.Time
	Log        *slog.Logger

	summaries sync.Mutex // one summary at a time, so two polls do not both pay for one
}

// Handler serves POST /research/lakes, GET /research/jobs and GET /research/jobs/{id}.
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /research/lakes", s.create)
	mux.HandleFunc("GET /research/jobs", s.list)
	mux.HandleFunc("GET /research/jobs/{id}", s.get)
	return mux
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Service) log() *slog.Logger {
	if s.Log == nil {
		return slog.New(slog.DiscardHandler)
	}
	return s.Log
}

type createRequest struct {
	Name        string   `json:"name"`
	Lat         *float64 `json:"lat"`
	Lon         *float64 `json:"lon"`
	Years       string   `json:"years"` // "2021-2026" or "2025"
	NotifyEmail string   `json:"notify_email"`
}

type jobView struct {
	*Job
	LogTail string `json:"log_tail,omitempty"`
}

func (s *Service) create(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		return
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	now := s.now()
	job, err := newJob(req, now)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if job.Lake == nil {
		lake, err := s.resolve(r.Context(), job.Name)
		switch {
		case errors.Is(err, ErrUnresolved):
			// Kept as a failed job, so the request and why it went nowhere stay on record.
			job.Status, job.Error, job.FinishedAt = StatusFailed, ErrUnresolved.Error(), job.CreatedAt
		case err != nil:
			s.log().ErrorContext(r.Context(), "research: resolving lake failed", "name", job.Name, "err", err)
			writeError(w, http.StatusBadGateway, "could not look the name up right now; try again, or send lat and lon")
			return
		default:
			job.Lake = lake
		}
	}
	if err := s.Store.Put(job); err != nil {
		s.log().ErrorContext(r.Context(), "research: writing job failed", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if job.Status == StatusFailed {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"job_id": job.ID, "status": job.Status, "error": job.Error})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job_id": job.ID, "status": job.Status, "lake": job.Lake})
}

func (s *Service) resolve(ctx context.Context, name string) (*Lake, error) {
	if s.Resolver == nil {
		return nil, ErrUnresolved
	}
	ctx, cancel := context.WithTimeout(ctx, resolveTimeout)
	defer cancel()
	lake, err := s.Resolver.Resolve(ctx, name)
	if err != nil {
		return nil, err
	}
	if lake == nil || !validPosition(lake.Lat, lake.Lon) {
		return nil, ErrUnresolved
	}
	return lake, nil
}

func validPosition(lat, lon float64) bool {
	return lat >= -90 && lat <= 90 && lon >= -180 && lon <= 180 && !(lat == 0 && lon == 0)
}

// newJob validates a request into a queued job. The lake is set only if coordinates were sent.
func newJob(req createRequest, now time.Time) (*Job, error) {
	name := strings.Join(strings.Fields(req.Name), " ")
	switch {
	case name == "":
		return nil, errors.New("name is required")
	case utf8.RuneCountInString(name) > maxNameChars:
		return nil, fmt.Errorf("name is longer than %d characters", maxNameChars)
	case strings.ContainsFunc(name, func(r rune) bool { return r < 0x20 || r == 0x7f }):
		return nil, errors.New("name contains control characters")
	case (req.Lat == nil) != (req.Lon == nil):
		return nil, errors.New("send both lat and lon, or neither")
	}
	job := &Job{ID: NewID(now), Status: StatusQueued, Name: name, CreatedAt: now.UTC().Format(time.RFC3339)}
	if req.Lat != nil {
		if !validPosition(*req.Lat, *req.Lon) {
			return nil, errors.New("lat must be within -90..90 and lon within -180..180")
		}
		job.Lake = &Lake{Name: name, Lat: *req.Lat, Lon: *req.Lon, Source: "request"}
	}
	var err error
	if job.YearFrom, job.YearTo, err = parseYears(req.Years, now.UTC().Year()); err != nil {
		return nil, err
	}
	if req.NotifyEmail != "" {
		addr, err := mail.ParseAddress(req.NotifyEmail)
		if err != nil || addr.Address != strings.TrimSpace(req.NotifyEmail) {
			return nil, errors.New("notify_email is not an email address")
		}
		job.NotifyEmail = addr.Address
	}
	return job, nil
}

// parseYears reads "2021-2026" or "2025"; empty means the current season and the five before.
func parseYears(s string, thisYear int) (from, to int, err error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return thisYear - defaultYears + 1, thisYear, nil
	}
	a, b, isRange := strings.Cut(s, "-")
	if from, err = strconv.Atoi(strings.TrimSpace(a)); err == nil {
		to = from
		if isRange {
			to, err = strconv.Atoi(strings.TrimSpace(b))
		}
	}
	switch {
	case err != nil:
		return 0, 0, errors.New(`years must look like "2021-2026" or "2025"`)
	case from > to:
		return 0, 0, errors.New("years: the first year is after the last")
	case from < firstYear || to > thisYear:
		return 0, 0, fmt.Errorf("years must be within %d-%d", firstYear, thisYear)
	case to-from+1 > maxYears:
		return 0, 0, fmt.Errorf("years: at most %d seasons per job", maxYears)
	}
	return from, to, nil
}

func (s *Service) list(w http.ResponseWriter, r *http.Request) {
	jobs, err := s.Store.List()
	if err != nil {
		s.log().ErrorContext(r.Context(), "research: listing jobs failed", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": jobs})
}

func (s *Service) get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	job, err := s.Store.Get(id)
	if errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusNotFound, "research job not found")
		return
	}
	if err != nil {
		s.log().ErrorContext(r.Context(), "research: reading job failed", "id", id, "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if job.Status == StatusDone && job.Summary == "" && s.Summarizer != nil {
		job = s.summarize(r.Context(), job)
	}
	writeJSON(w, http.StatusOK, jobView{Job: job, LogTail: s.Store.LogTail(id)})
}

// summarize adds the summary to a finished job and saves it. A failure leaves the job as it is;
// the next read tries again.
func (s *Service) summarize(ctx context.Context, job *Job) *Job {
	s.summaries.Lock()
	defer s.summaries.Unlock()
	if fresh, err := s.Store.Get(job.ID); err == nil { // another request may have just written it
		job = fresh
	}
	if job.Status != StatusDone || job.Summary != "" {
		return job
	}
	ctx, cancel := context.WithTimeout(ctx, summaryTimeout)
	defer cancel()
	text, mode, err := s.Summarizer.Summarize(ctx, job)
	if err != nil || text == "" {
		s.log().WarnContext(ctx, "research: no summary", "id", job.ID, "err", err)
		return job
	}
	job.Summary, job.SummaryMode = text, mode
	if err := s.Store.Put(job); err != nil {
		s.log().ErrorContext(ctx, "research: saving summary failed", "id", job.ID, "err", err)
	}
	return job
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
	writeJSON(w, status, map[string]any{"error": msg})
}
