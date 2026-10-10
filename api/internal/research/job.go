// Package research lets officials ask for a first analysis of a lake that is not monitored yet:
// the API resolves the lake's position and queues a job file; a worker on the server
// (infra/ec2/research-worker.sh) runs the pipeline on it and writes the results back into the
// file. The API itself only reads and writes job files.
package research

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

// Job statuses. The API writes queued (or failed, when the lake cannot be placed); the worker
// moves a job through running to done or failed.
const (
	StatusQueued  = "queued"
	StatusRunning = "running"
	StatusDone    = "done"
	StatusFailed  = "failed"
)

// Lake is where the analysis is centred and how that position was found.
type Lake struct {
	Name string  `json:"name"`
	Lat  float64 `json:"lat"`
	Lon  float64 `json:"lon"`
	// Source is "request" (coordinates sent by the caller), "catalogue:<lake id>" or
	// "places:<osm id>" (found by name).
	Source string `json:"source"`
}

// Job is one job file, <jobs dir>/<id>.json. The worker edits status, started_at, finished_at,
// error and results in place; everything else is written by the API.
type Job struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	Name        string `json:"name"` // as asked for
	Lake        *Lake  `json:"lake"`
	YearFrom    int    `json:"year_from"`
	YearTo      int    `json:"year_to"`
	NotifyEmail string `json:"notify_email,omitempty"`
	CreatedAt   string `json:"created_at"`
	StartedAt   string `json:"started_at,omitempty"`
	FinishedAt  string `json:"finished_at,omitempty"`
	Error       string `json:"error,omitempty"`
	// Results is written by the worker: {dir, s3_prefix?, files, snapped, series, risk}.
	Results json.RawMessage `json:"results,omitempty"`
	// Summary is a short plain-language reading of the results, added by the API once the job is
	// done; SummaryMode says whether the model wrote it ("model") or a fixed template ("template").
	Summary     string `json:"summary,omitempty"`
	SummaryMode string `json:"summary_mode,omitempty"`
}

// ErrNotFound: no job with that id.
var ErrNotFound = errors.New("research job not found")

// Store keeps jobs (the jobs directory; a fake in tests).
type Store interface {
	Put(job *Job) error
	Get(id string) (*Job, error)
	// List returns every job, newest first.
	List() ([]*Job, error)
	// LogTail returns the end of the worker's log for a job, "" if there is none yet.
	LogTail(id string) string
}

var jobID = regexp.MustCompile(`^r-[0-9]{8}-[0-9a-f]{8}$`)

// ValidID reports whether id has the shape NewID gives; nothing else is ever used in a file name.
func ValidID(id string) bool { return jobID.MatchString(id) }

// NewID returns a new job id: the day it was created plus a random part.
func NewID(now time.Time) string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err) // crypto/rand does not fail on supported platforms
	}
	return "r-" + now.UTC().Format("20060102") + "-" + hex.EncodeToString(b[:])
}

const logTailBytes = 2000

// DirStore keeps each job as <Dir>/<id>.json, with the worker's log beside it as <id>.log.
type DirStore struct {
	Dir string
}

func NewDirStore(dir string) (*DirStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &DirStore{Dir: dir}, nil
}

// Put writes the job file atomically, so the worker never sees half a file.
func (s *DirStore) Put(job *Job) error {
	if !ValidID(job.ID) {
		return fmt.Errorf("invalid job id %q", job.ID)
	}
	b, err := json.MarshalIndent(job, "", " ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.Dir, ".job-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(append(b, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(s.Dir, job.ID+".json"))
}

func (s *DirStore) Get(id string) (*Job, error) {
	if !ValidID(id) {
		return nil, ErrNotFound
	}
	b, err := os.ReadFile(filepath.Join(s.Dir, id+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var job Job
	if err := json.Unmarshal(b, &job); err != nil {
		return nil, fmt.Errorf("job %s: %w", id, err)
	}
	job.ID = id
	return &job, nil
}

func (s *DirStore) List() ([]*Job, error) {
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		return nil, err
	}
	jobs := []*Job{}
	for _, e := range entries {
		id, ok := strings.CutSuffix(e.Name(), ".json")
		if !ok || !ValidID(id) {
			continue
		}
		job, err := s.Get(id)
		if err != nil {
			continue // unreadable or mid-rewrite: leave it out of the list
		}
		jobs = append(jobs, job)
	}
	slices.SortStableFunc(jobs, func(a, b *Job) int {
		if c := strings.Compare(b.CreatedAt, a.CreatedAt); c != 0 {
			return c
		}
		return strings.Compare(b.ID, a.ID)
	})
	return jobs, nil
}

func (s *DirStore) LogTail(id string) string {
	if !ValidID(id) {
		return ""
	}
	f, err := os.Open(filepath.Join(s.Dir, id+".log"))
	if err != nil {
		return ""
	}
	defer f.Close()
	if st, err := f.Stat(); err == nil && st.Size() > logTailBytes {
		f.Seek(-logTailBytes, io.SeekEnd)
	}
	b, err := io.ReadAll(io.LimitReader(f, logTailBytes))
	if err != nil {
		return ""
	}
	tail := strings.ToValidUTF8(string(b), "")
	if len(b) == logTailBytes { // started mid-line: drop the partial first line
		if _, rest, ok := strings.Cut(tail, "\n"); ok {
			tail = rest
		}
	}
	return strings.TrimRight(tail, "\n")
}
