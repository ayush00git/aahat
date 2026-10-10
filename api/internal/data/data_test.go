package data

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLocalStoreStaysInRoot(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "root", "lakes"), 0o755)
	os.WriteFile(filepath.Join(dir, "secret.txt"), []byte("s"), 0o644)
	os.WriteFile(filepath.Join(dir, "root", "lakes", "index.json"), []byte("{}"), 0o644)
	os.Symlink(filepath.Join(dir, "secret.txt"), filepath.Join(dir, "root", "link.txt"))

	s, err := NewLocalStore(filepath.Join(dir, "root"))
	if err != nil {
		t.Fatal(err)
	}
	if b, err := s.Get(context.Background(), "lakes/index.json"); err != nil || string(b) != "{}" {
		t.Fatalf("Get = %q, %v", b, err)
	}
	for _, key := range []string{"../secret.txt", "/etc/passwd", "lakes/../../secret.txt", "missing.json"} {
		if _, err := s.Get(context.Background(), key); !errors.Is(err, ErrNotFound) {
			t.Errorf("Get(%q) err = %v, want ErrNotFound", key, err)
		}
	}
	if _, err := s.Get(context.Background(), "link.txt"); err == nil {
		t.Error("symlink out of the root was followed")
	}
}

type countingStore struct{ n int }

func (c *countingStore) Get(context.Context, string) ([]byte, error) {
	c.n++
	return []byte("x"), nil
}

func TestCacheTTL(t *testing.T) {
	src := &countingStore{}
	c := NewCache(src, time.Minute)
	now := time.Unix(0, 0)
	c.now = func() time.Time { return now }
	ctx := context.Background()

	c.Get(ctx, "k")
	c.Get(ctx, "k")
	if src.n != 1 {
		t.Fatalf("fetched %d times within TTL", src.n)
	}
	now = now.Add(2 * time.Minute)
	c.Get(ctx, "k")
	if src.n != 2 {
		t.Fatalf("fetched %d times after TTL", src.n)
	}
}

func TestLayerFile(t *testing.T) {
	for name, want := range map[string]string{
		"flood_path": "flood_path.geojson", "outlet.geojson": "outlet.geojson", "2024": "2024.geojson",
		"impacts": "", "../index": "", "20245": "", "": "", "risk.json": "",
	} {
		got, ok := LayerFile(name)
		if ok != (want != "") || got != want {
			t.Errorf("LayerFile(%q) = %q, %v", name, got, ok)
		}
	}
}

func TestAffectedUnder(t *testing.T) {
	out := func(s string) *Outcome { return &Outcome{Status: s} }
	tests := []struct {
		name             string
		im               Impact
		expected, severe bool
	}{
		{"flooded both", Impact{Status: StatusInFloodPath, Scenarios: &Scenarios{out("flooded"), out("flooded")}}, true, true},
		{"severe only", Impact{Status: StatusAtRisk, Scenarios: &Scenarios{out("outside"), out("margin")}}, false, true},
		{"margin both", Impact{Status: StatusAtRisk, Scenarios: &Scenarios{out("margin"), out("margin")}}, true, true},
		{"outside", Impact{Status: StatusOutside, Scenarios: &Scenarios{out("outside"), out("outside")}}, false, false},
		{"no scenarios", Impact{Status: StatusAtRisk}, true, true},
	}
	for _, tc := range tests {
		if got := tc.im.AffectedUnder(ScenarioExpected); got != tc.expected {
			t.Errorf("%s: expected = %v", tc.name, got)
		}
		if got := tc.im.AffectedUnder(ScenarioSevere); got != tc.severe {
			t.Errorf("%s: severe = %v", tc.name, got)
		}
	}
}

type stubStore struct {
	body []byte
	err  error
	gets int
}

func (s *stubStore) Get(context.Context, string) ([]byte, error) {
	s.gets++
	return s.body, s.err
}

func TestFallback(t *testing.T) {
	ctx := context.Background()
	local := &stubStore{body: []byte("local")}

	ok := &stubStore{body: []byte("s3")}
	if b, err := NewFallback(ok, local, nil).Get(ctx, "k"); err != nil || string(b) != "s3" || local.gets != 0 {
		t.Fatalf("healthy primary: %q, %v, local reads %d", b, err, local.gets)
	}
	// A key missing from the primary is missing: the local copy must not resurrect a deleted file.
	missing := &stubStore{err: ErrNotFound}
	if _, err := NewFallback(missing, local, nil).Get(ctx, "k"); !errors.Is(err, ErrNotFound) || local.gets != 0 {
		t.Fatalf("missing key: %v, local reads %d", err, local.gets)
	}
	broken := &stubStore{err: errors.New("s3 unreachable")}
	if b, err := NewFallback(broken, local, nil).Get(ctx, "k"); err != nil || string(b) != "local" {
		t.Fatalf("broken primary: %q, %v", b, err)
	}
}
