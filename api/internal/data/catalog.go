package data

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

const indexKey = "lakes/index.json"

// Catalog gives typed access to the published files. It parses on every call;
// the underlying DataStore is expected to be cached.
type Catalog struct {
	src DataStore
}

func NewCatalog(src DataStore) *Catalog { return &Catalog{src: src} }

// IndexJSON returns lakes/index.json as published.
func (c *Catalog) IndexJSON(ctx context.Context) ([]byte, error) {
	return c.src.Get(ctx, indexKey)
}

func (c *Catalog) Index(ctx context.Context) (*Index, error) {
	b, err := c.IndexJSON(ctx)
	if err != nil {
		return nil, fmt.Errorf("read index: %w", err)
	}
	var doc struct {
		GeneratedAt string            `json:"generated_at"`
		Lakes       []json.RawMessage `json:"lakes"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("parse index: %w", err)
	}
	idx := &Index{GeneratedAt: doc.GeneratedAt, Lakes: make([]Lake, 0, len(doc.Lakes))}
	for _, raw := range doc.Lakes {
		var l Lake
		if err := json.Unmarshal(raw, &l); err != nil {
			return nil, fmt.Errorf("parse index entry: %w", err)
		}
		l.Raw = raw
		idx.Lakes = append(idx.Lakes, l)
	}
	return idx, nil
}

// Lake returns one index entry, or ErrNotFound. Only lakes listed in the index
// are reachable, which also keeps arbitrary ids out of storage keys.
func (c *Catalog) Lake(ctx context.Context, id string) (*Lake, error) {
	idx, err := c.Index(ctx)
	if err != nil {
		return nil, err
	}
	for i := range idx.Lakes {
		if idx.Lakes[i].ID == id {
			return &idx.Lakes[i], nil
		}
	}
	return nil, ErrNotFound
}

// LakeFile returns lakes/<id>/<name> for a lake in the index.
func (c *Catalog) LakeFile(ctx context.Context, id, name string) ([]byte, error) {
	if _, err := c.Lake(ctx, id); err != nil {
		return nil, err
	}
	return c.src.Get(ctx, "lakes/"+id+"/"+name)
}

// Impacts returns a lake's impacts.json rows in file order (nearest first).
func (c *Catalog) Impacts(ctx context.Context, id string) ([]Impact, error) {
	b, err := c.LakeFile(ctx, id, "impacts.json")
	if err != nil {
		return nil, err
	}
	return parseImpacts(b)
}

func parseImpacts(b []byte) ([]Impact, error) {
	var raws []json.RawMessage
	if err := json.Unmarshal(b, &raws); err != nil {
		return nil, fmt.Errorf("parse impacts: %w", err)
	}
	out := make([]Impact, len(raws))
	for i, raw := range raws {
		if err := json.Unmarshal(raw, &out[i]); err != nil {
			return nil, fmt.Errorf("parse impact %d: %w", i, err)
		}
		out[i].Raw = raw
	}
	return out, nil
}

// LakeImpacts pairs a lake with its impacts.
type LakeImpacts struct {
	Lake    Lake
	Impacts []Impact
}

// AllImpacts returns every indexed lake's impacts, in index order. Lakes whose
// impacts.json is missing (pipeline not run that far yet) are skipped.
func (c *Catalog) AllImpacts(ctx context.Context) ([]LakeImpacts, error) {
	idx, err := c.Index(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]LakeImpacts, 0, len(idx.Lakes))
	for _, l := range idx.Lakes {
		b, err := c.src.Get(ctx, "lakes/"+l.ID+"/impacts.json")
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		imp, err := parseImpacts(b)
		if err != nil {
			return nil, fmt.Errorf("lake %s: %w", l.ID, err)
		}
		out = append(out, LakeImpacts{Lake: l, Impacts: imp})
	}
	return out, nil
}

var (
	namedLayers = map[string]bool{
		"outlines": true, "outlet": true, "glaciers": true, "flood_path": true,
		"corridor_expected": true, "corridor_severe": true,
	}
	yearLayer = regexp.MustCompile(`^(19|20)\d{2}$`)
)

// LayerFile maps a layer name ("flood_path", "2024", optionally with a
// ".geojson" suffix) to its file name, or reports false if it is not one of
// the published layers.
func LayerFile(name string) (string, bool) {
	name = strings.TrimSuffix(name, ".geojson")
	if namedLayers[name] || yearLayer.MatchString(name) {
		return name + ".geojson", true
	}
	return "", false
}
