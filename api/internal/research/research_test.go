package research_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"

	"github.com/ayush00git/aahat/api/internal/assistant"
	"github.com/ayush00git/aahat/api/internal/data"
	"github.com/ayush00git/aahat/api/internal/research"
)

var testNow = time.Date(2026, 10, 10, 6, 0, 0, 0, time.UTC)

// memStore is a Store without a directory.
type memStore struct {
	jobs map[string]*research.Job
	logs map[string]string
	puts int
}

func newMemStore() *memStore {
	return &memStore{jobs: map[string]*research.Job{}, logs: map[string]string{}}
}

func (m *memStore) Put(job *research.Job) error {
	cp := *job
	m.jobs[job.ID] = &cp
	m.puts++
	return nil
}

func (m *memStore) Get(id string) (*research.Job, error) {
	job, ok := m.jobs[id]
	if !ok {
		return nil, research.ErrNotFound
	}
	cp := *job
	return &cp, nil
}

func (m *memStore) List() ([]*research.Job, error) {
	out := []*research.Job{}
	for _, j := range m.jobs {
		out = append(out, j)
	}
	return out, nil
}

func (m *memStore) LogTail(id string) string { return m.logs[id] }

type fakeResolver struct {
	lake  *research.Lake
	err   error
	asked []string
}

func (f *fakeResolver) Resolve(_ context.Context, name string) (*research.Lake, error) {
	f.asked = append(f.asked, name)
	return f.lake, f.err
}

type fakeSummarizer struct{ calls int }

func (f *fakeSummarizer) Summarize(context.Context, *research.Job) (string, string, error) {
	f.calls++
	return "The lake was steady.", assistant.ModeModel, nil
}

func do(t *testing.T, h http.Handler, method, target string, body any) (int, map[string]any) {
	t.Helper()
	var rd bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&rd).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, target, &rd))
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("%s %s: body is not a JSON object: %s", method, target, rec.Body)
	}
	return rec.Code, out
}

func TestCreateWithCoordinatesQueuesAJob(t *testing.T) {
	st := newMemStore()
	resolver := &fakeResolver{}
	svc := &research.Service{Store: st, Resolver: resolver, Now: func() time.Time { return testNow }}
	code, out := do(t, svc.Handler(), "POST", "/research/lakes", map[string]any{
		"name": "  Bhrigu   Lake ", "lat": 32.29333, "lon": 77.24248, "notify_email": "ddma@example.org",
	})
	if code != http.StatusAccepted || out["status"] != "queued" {
		t.Fatalf("status %d, body %v", code, out)
	}
	id, _ := out["job_id"].(string)
	if !research.ValidID(id) || !strings.HasPrefix(id, "r-20261010-") {
		t.Fatalf("job_id = %q", id)
	}
	job := st.jobs[id]
	if job == nil || job.Status != research.StatusQueued || job.Name != "Bhrigu Lake" || job.NotifyEmail != "ddma@example.org" {
		t.Fatalf("stored job = %+v", job)
	}
	if job.Lake == nil || job.Lake.Lat != 32.29333 || job.Lake.Lon != 77.24248 || job.Lake.Source != "request" {
		t.Fatalf("lake = %+v", job.Lake)
	}
	if job.YearFrom != 2021 || job.YearTo != 2026 { // the default: this season and the five before
		t.Fatalf("years = %d-%d", job.YearFrom, job.YearTo)
	}
	if len(resolver.asked) != 0 {
		t.Fatalf("resolver was asked although coordinates were sent: %v", resolver.asked)
	}
}

func TestCreateByNameUsesTheResolver(t *testing.T) {
	st := newMemStore()
	resolver := &fakeResolver{lake: &research.Lake{Name: "Chandra Tal", Lat: 32.4825, Lon: 77.6146, Source: "catalogue:chandra-tal"}}
	svc := &research.Service{Store: st, Resolver: resolver, Now: func() time.Time { return testNow }}
	code, out := do(t, svc.Handler(), "POST", "/research/lakes", map[string]any{"name": "Chandratal", "years": "2024-2025"})
	if code != http.StatusAccepted {
		t.Fatalf("status %d, body %v", code, out)
	}
	job := st.jobs[out["job_id"].(string)]
	if job.Lake.Source != "catalogue:chandra-tal" || job.Lake.Lat != 32.4825 || job.YearFrom != 2024 || job.YearTo != 2025 {
		t.Fatalf("job = %+v lake = %+v", job, job.Lake)
	}

	// Unresolved: the job is kept as failed, with a message asking for coordinates.
	resolver.lake, resolver.err = nil, research.ErrUnresolved
	code, out = do(t, svc.Handler(), "POST", "/research/lakes", map[string]any{"name": "Some Unknown Tarn"})
	if code != http.StatusUnprocessableEntity || out["status"] != "failed" || !strings.Contains(out["error"].(string), "coordinates") {
		t.Fatalf("status %d, body %v", code, out)
	}
	failed := st.jobs[out["job_id"].(string)]
	if failed == nil || failed.Status != research.StatusFailed || failed.Lake != nil || failed.Error == "" {
		t.Fatalf("failed job = %+v", failed)
	}

	// The lookup itself breaking is not the caller's fault and leaves no job behind.
	before := len(st.jobs)
	resolver.err = errors.New("bedrock unreachable")
	if code, _ = do(t, svc.Handler(), "POST", "/research/lakes", map[string]any{"name": "Some Lake"}); code != http.StatusBadGateway || len(st.jobs) != before {
		t.Fatalf("status %d, jobs %d -> %d", code, before, len(st.jobs))
	}

	// No resolver at all: a name alone cannot be placed.
	bare := &research.Service{Store: st, Now: func() time.Time { return testNow }}
	if code, out = do(t, bare.Handler(), "POST", "/research/lakes", map[string]any{"name": "Some Lake"}); code != http.StatusUnprocessableEntity {
		t.Fatalf("status %d, body %v", code, out)
	}
}

func TestCreateValidation(t *testing.T) {
	svc := &research.Service{Store: newMemStore(), Now: func() time.Time { return testNow }}
	for name, body := range map[string]map[string]any{
		"no name":        {"lat": 32.0, "lon": 77.0},
		"lat only":       {"name": "x lake", "lat": 32.0},
		"lat range":      {"name": "x lake", "lat": 132.0, "lon": 77.0},
		"years order":    {"name": "x lake", "lat": 32.0, "lon": 77.0, "years": "2026-2021"},
		"years future":   {"name": "x lake", "lat": 32.0, "lon": 77.0, "years": "2025-2027"},
		"years too old":  {"name": "x lake", "lat": 32.0, "lon": 77.0, "years": "2010-2012"},
		"years too many": {"name": "x lake", "lat": 32.0, "lon": 77.0, "years": "2016-2026"},
		"years garbage":  {"name": "x lake", "lat": 32.0, "lon": 77.0, "years": "recent"},
		"bad email":      {"name": "x lake", "lat": 32.0, "lon": 77.0, "notify_email": "not an email"},
	} {
		if code, out := do(t, svc.Handler(), "POST", "/research/lakes", body); code != http.StatusBadRequest || out["error"] == "" {
			t.Errorf("%s: status %d, body %v", name, code, out)
		}
	}
}

const doneResults = `{"dir":"/srv/aahat/research/x","files":["series.json","risk.json"],
 "snapped":{"lat":32.2933,"lon":77.2425,"area_km2":0.0094,"elev_m":4235,"distance_m":6},
 "series":[{"year":2025,"status":"ok","area_m2":9400.0,"uncertainty_m2":3600.0},{"year":2026,"status":"partial","area_m2":9100.0,"uncertainty_m2":3500.0}],
 "risk":{"score":4.2,"level":"low","data_until":"2026-10-05","factors":[{"key":"volume","value":45310.5,"unit":"m³"}]}}`

func doneJob(id string) *research.Job {
	return &research.Job{
		ID: id, Status: research.StatusDone, Name: "Bhrigu Lake", YearFrom: 2025, YearTo: 2026,
		Lake:    &research.Lake{Name: "Bhrigu Lake", Lat: 32.29333, Lon: 77.24248, Source: "request"},
		Results: json.RawMessage(doneResults), CreatedAt: "2026-10-10T06:00:00Z",
	}
}

func TestGetJobAddsTheSummaryOnce(t *testing.T) {
	st := newMemStore()
	const id = "r-20261010-00000001"
	st.jobs[id] = doneJob(id)
	st.logs[id] = "series 2026 ok"
	sum := &fakeSummarizer{}
	svc := &research.Service{Store: st, Summarizer: sum}

	for range 2 {
		code, out := do(t, svc.Handler(), "GET", "/research/jobs/"+id, nil)
		if code != http.StatusOK || out["status"] != "done" || out["summary"] != "The lake was steady." || out["log_tail"] != "series 2026 ok" {
			t.Fatalf("status %d, body %v", code, out)
		}
		lake := out["lake"].(map[string]any)
		res := out["results"].(map[string]any)
		if lake["source"] != "request" || len(res["files"].([]any)) != 2 || len(res["series"].([]any)) != 2 {
			t.Fatalf("lake %v results %v", lake, res)
		}
	}
	if sum.calls != 1 || st.jobs[id].Summary == "" {
		t.Fatalf("summarizer called %d times; stored summary %q", sum.calls, st.jobs[id].Summary)
	}

	// A job that is not finished gets no summary; unknown and malformed ids are 404.
	const running = "r-20261010-00000002"
	st.jobs[running] = &research.Job{ID: running, Status: research.StatusRunning, Name: "x"}
	if _, out := do(t, svc.Handler(), "GET", "/research/jobs/"+running, nil); out["summary"] != nil || sum.calls != 1 {
		t.Fatalf("running job: %v", out)
	}
	for _, bad := range []string{"r-20261010-ffffffff", "..%2Fstore"} {
		if code, _ := do(t, svc.Handler(), "GET", "/research/jobs/"+bad, nil); code != http.StatusNotFound {
			t.Errorf("%s: status %d", bad, code)
		}
	}
	code, out := do(t, svc.Handler(), "GET", "/research/jobs", nil)
	if code != http.StatusOK || len(out["jobs"].([]any)) != 2 {
		t.Fatalf("list: status %d, body %v", code, out)
	}
}

func TestDirStore(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "jobs")
	st, err := research.NewDirStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	old, recent := doneJob("r-20261009-0000000a"), doneJob("r-20261010-0000000b")
	old.CreatedAt = "2026-10-09T06:00:00Z"
	for _, j := range []*research.Job{old, recent} {
		if err := st.Put(j); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.Put(&research.Job{ID: "../escape"}); err == nil {
		t.Fatal("Put accepted a path as id")
	}
	// What the worker writes back is read as is: fields the API does not set survive a read.
	workerFile := `{"id":"r-20261010-0000000c","status":"running","name":"Lake C","lake":{"name":"Lake C","lat":32.1,"lon":77.1,"source":"request"},
	  "year_from":2025,"year_to":2026,"created_at":"2026-10-10T07:00:00Z","started_at":"2026-10-10T07:00:05Z"}`
	if err := os.WriteFile(filepath.Join(dir, "r-20261010-0000000c.json"), []byte(workerFile), 0o600); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "notes.json"), []byte("{}"), 0o600) // not a job: ignored
	os.WriteFile(filepath.Join(dir, ".lock"), nil, 0o600)

	jobs, err := st.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 3 || jobs[0].ID != "r-20261010-0000000c" || jobs[2].ID != old.ID || jobs[0].StartedAt == "" {
		t.Fatalf("list = %v", ids(jobs))
	}
	got, err := st.Get(recent.ID)
	if err != nil || got.Lake.Lat != 32.29333 || !bytes.Contains(got.Results, []byte(`"area_m2"`)) {
		t.Fatalf("get = %+v, %v", got, err)
	}
	if _, err := st.Get("r-20261010-ffffffff"); !errors.Is(err, research.ErrNotFound) {
		t.Fatalf("missing job: err = %v", err)
	}

	if st.LogTail(recent.ID) != "" {
		t.Fatal("log tail of a job without a log")
	}
	long := strings.Repeat("an early line of pipeline output\n", 200) + "the last line"
	os.WriteFile(filepath.Join(dir, recent.ID+".log"), []byte(long), 0o600)
	tail := st.LogTail(recent.ID)
	if !strings.HasSuffix(tail, "the last line") || len(tail) > 2000 || !strings.HasPrefix(tail, "an early line") {
		t.Fatalf("log tail (%d bytes) = %q", len(tail), tail)
	}
}

func ids(jobs []*research.Job) []string {
	var out []string
	for _, j := range jobs {
		out = append(out, j.ID)
	}
	return out
}

// fakeModel replays scripted Converse replies.
type fakeModel struct {
	replies []types.Message
	inputs  []*bedrockruntime.ConverseInput
}

func (f *fakeModel) Converse(_ context.Context, in *bedrockruntime.ConverseInput, _ ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseOutput, error) {
	f.inputs = append(f.inputs, in)
	if len(f.replies) == 0 {
		return nil, errors.New("fake model: no reply scripted")
	}
	msg := f.replies[0]
	f.replies = f.replies[1:]
	return &bedrockruntime.ConverseOutput{Output: &types.ConverseOutputMemberMessage{Value: msg}}, nil
}

func say(text string) types.Message {
	return types.Message{Role: types.ConversationRoleAssistant, Content: []types.ContentBlock{&types.ContentBlockMemberText{Value: text}}}
}

func use(tool string, args map[string]any) types.Message {
	return types.Message{Role: types.ConversationRoleAssistant, Content: []types.ContentBlock{
		&types.ContentBlockMemberToolUse{Value: types.ToolUseBlock{
			ToolUseId: aws.String("call-" + tool), Name: aws.String(tool), Input: document.NewLazyDocument(args),
		}},
	}}
}

func testIndex(t *testing.T) research.Index {
	t.Helper()
	local, err := data.NewLocalStore("../httpapi/testdata")
	if err != nil {
		t.Fatal(err)
	}
	return research.Index{Catalog: data.NewCatalog(local)}
}

func TestClaudeResolver(t *testing.T) {
	ctx := context.Background()
	// A name that is exactly one monitored lake needs no model call.
	model := &fakeModel{}
	r := research.ClaudeResolver{Client: model, Model: "m", Index: testIndex(t)}
	lake, err := r.Resolve(ctx, "gepang gath")
	if err != nil || lake.Source != "catalogue:gepang-gath" || lake.Lat != 32.5237 || lake.Lon != 77.2218 || len(model.inputs) != 0 {
		t.Fatalf("lake = %+v, err = %v, model calls = %d", lake, err, len(model.inputs))
	}

	// Otherwise the model searches and points at a result; the position comes from that result.
	model = &fakeModel{replies: []types.Message{
		use("find_water", map[string]any{"name": "Gepan Ghat Lake"}),
		use("find_water", map[string]any{"name": "Gepang"}),
		use("choose", map[string]any{"ref": "r1"}),
		say("Chosen."),
	}}
	r.Client = model
	lake, err = r.Resolve(ctx, "Gepan Ghat glacier lake")
	if err != nil || lake.Source != "catalogue:gepang-gath" || lake.Lat != 32.5237 || lake.Name != "Gepang Gath" {
		t.Fatalf("lake = %+v, err = %v", lake, err)
	}

	// A ref that no search returned is refused, and coordinates the model writes in prose are ignored.
	model = &fakeModel{replies: []types.Message{
		use("find_water", map[string]any{"name": "Hamirpur"}),
		use("choose", map[string]any{"ref": "r99"}),
		say("It is at 31.68, 76.52."),
	}}
	r.Client = model
	if lake, err = r.Resolve(ctx, "Hamirpur lake"); !errors.Is(err, research.ErrUnresolved) || lake != nil {
		t.Fatalf("lake = %+v, err = %v", lake, err)
	}

	model = &fakeModel{replies: []types.Message{
		use("find_water", map[string]any{"name": "Nowhere"}),
		use("give_up", map[string]any{"reason": "no match"}),
		say("Not found."),
	}}
	r.Client = model
	if _, err = r.Resolve(ctx, "Nowhere Tso"); !errors.Is(err, research.ErrUnresolved) {
		t.Fatalf("err = %v", err)
	}

	// The model being unreachable is a different error from "not found".
	r.Client = &fakeModel{}
	if _, err = r.Resolve(ctx, "Some Lake"); err == nil || errors.Is(err, research.ErrUnresolved) {
		t.Fatalf("err = %v", err)
	}

	// Without a model only exact catalogue names resolve.
	plain := research.CatalogueResolver{Index: testIndex(t)}
	if _, err = plain.Resolve(ctx, "Gepan Ghat"); !errors.Is(err, research.ErrUnresolved) {
		t.Fatalf("err = %v", err)
	}
}

func TestClaudeSummarizerChecksNumbers(t *testing.T) {
	ctx := context.Background()
	job := doneJob("r-20261010-00000001")

	good := "Bhrigu Lake measured 9400 m² in 2025 and 9100 m² in 2026, steady within uncertainty. The screening score is 4.2 out of 100 (low)."
	model := &fakeModel{replies: []types.Message{say(good)}}
	text, mode, err := research.ClaudeSummarizer{Client: model, Model: "m"}.Summarize(ctx, job)
	if err != nil || mode != assistant.ModeModel || text != good {
		t.Fatalf("text = %q, mode = %q, err = %v", text, mode, err)
	}
	sent := model.inputs[0].Messages[0].Content[0].(*types.ContentBlockMemberText).Value
	if !strings.Contains(sent, `"area_m2":9400`) || strings.Contains(sent, "/srv/aahat") {
		t.Fatalf("facts sent to the model: %s", sent)
	}

	// A computed figure (a 3% drop) is rejected; the corrected retry is accepted.
	model = &fakeModel{replies: []types.Message{say("The lake shrank by 3% to 9100 m²."), say("The lake shrank slightly to 9100 m² in 2026.")}}
	text, mode, err = research.ClaudeSummarizer{Client: model, Model: "m"}.Summarize(ctx, job)
	if err != nil || mode != assistant.ModeModel || strings.Contains(text, "3%") || len(model.inputs) != 2 {
		t.Fatalf("text = %q, mode = %q, err = %v", text, mode, err)
	}

	// Two bad answers: the template, built from the data alone.
	model = &fakeModel{replies: []types.Message{say("It shrank by 3%."), say("It shrank by about 300 m².")}}
	text, mode, err = research.ClaudeSummarizer{Client: model, Model: "m"}.Summarize(ctx, job)
	if err != nil || mode != assistant.ModeTemplate {
		t.Fatalf("mode = %q, err = %v", mode, err)
	}
	for _, want := range []string{"Bhrigu Lake", "2025: 9400 m²", "2026: 9100 m² (partial)", "score 4.2 (low)", "2026-10-05"} {
		if !strings.Contains(text, want) {
			t.Errorf("template %q lacks %q", text, want)
		}
	}
}
