package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ayush00git/aahat/api/internal/alert"
	"github.com/ayush00git/aahat/api/internal/data"
	"github.com/ayush00git/aahat/api/internal/store"
)

const testSecret = "test-secret"

type testEnv struct {
	handler  http.Handler
	notifier *alert.LogNotifier
	store    *store.FileStore
}

// newEnv serves testdata/ through the full handler stack. edit, if given,
// can rewrite a fixture file before it is served.
func newEnv(t *testing.T, edit func(key string, body []byte) []byte) *testEnv {
	t.Helper()
	local, err := data.NewLocalStore("testdata")
	if err != nil {
		t.Fatal(err)
	}
	var src data.DataStore = local
	if edit != nil {
		src = editStore{local, edit}
	}
	catalog := data.NewCatalog(src)
	st := store.NewMemoryStore()
	n := alert.NewLogNotifier(nil)
	svc := alert.NewService(catalog, st, st, alert.NewDispatcher(n))
	h := New(Config{Catalog: catalog, Store: st, Alerts: svc, WebhookSecret: []byte(testSecret)})
	return &testEnv{handler: h, notifier: n, store: st}
}

type editStore struct {
	next data.DataStore
	edit func(string, []byte) []byte
}

func (e editStore) Get(ctx context.Context, key string) ([]byte, error) {
	b, err := e.next.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	return e.edit(key, b), nil
}

type response struct {
	code   int
	header http.Header
	body   []byte
}

func (env *testEnv) do(t *testing.T, method, target string, body any, headers ...string) response {
	t.Helper()
	var rd io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		rd = bytes.NewBufferString(b)
	case []byte:
		rd = bytes.NewReader(b)
	default:
		js, err := json.Marshal(b)
		if err != nil {
			t.Fatal(err)
		}
		rd = bytes.NewReader(js)
	}
	req := httptest.NewRequest(method, target, rd)
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	env.handler.ServeHTTP(rec, req)
	return response{code: rec.Code, header: rec.Header(), body: rec.Body.Bytes()}
}

func decode[T any](t *testing.T, r response) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(r.body, &v); err != nil {
		t.Fatalf("decode %s: %v", r.body, err)
	}
	return v
}

func expectStatus(t *testing.T, r response, want int) {
	t.Helper()
	if r.code != want {
		t.Fatalf("status = %d, want %d; body: %s", r.code, want, r.body)
	}
}

// expectError checks the status and that the body is {"error": "..."}.
func expectError(t *testing.T, r response, want int) {
	t.Helper()
	expectStatus(t, r, want)
	e := decode[map[string]string](t, r)
	if e["error"] == "" {
		t.Fatalf("no error message in %s", r.body)
	}
}

func readFixture(t *testing.T, rel string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", rel))
	if err != nil {
		t.Fatal(err)
	}
	return b
}
